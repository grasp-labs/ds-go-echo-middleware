package middleware

import (
	"bytes"
	"encoding/json"
	"io"

	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/internal/utils"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/adapters"
	ctx "github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/claims"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/requestctx"
)

// -------- helpers --------

func isJSON(ct string) bool {
	if ct == "" {
		return false
	}
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(strings.ToLower(ct))
	return ct == "application/json" || strings.HasSuffix(ct, "+json")
}

const defaultAuditMaxBodyBytes = 64 << 10

type auditOptions struct {
	maxBodyBytes   int
	requestPayload func(c echo.Context, body []byte) json.RawMessage
}

// AuditOption configures AuditMiddleware.
type AuditOption func(*auditOptions)

// WithMaxBodyBytes caps how many request and error-response bytes are buffered for the audit event (default 64 KiB).
// Request bodies exceeding the cap are passed through untouched and omitted from the event.
func WithMaxBodyBytes(n int) AuditOption {
	return func(o *auditOptions) { o.maxBodyBytes = n }
}

// WithRequestPayload replaces the captured request body. Returning nil drops it.
// f receives the complete body of JSON requests on mutating methods, never a truncated one.
func WithRequestPayload(f func(c echo.Context, body []byte) json.RawMessage) AuditOption {
	return func(o *auditOptions) { o.requestPayload = f }
}

// responseWriter copies up to limit bytes of the response body, only for error statuses.
type responseWriter struct {
	http.ResponseWriter
	status int
	limit  int
	body   bytes.Buffer
}

func (w *responseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if w.status >= 400 {
		if room := w.limit - w.body.Len(); room > 0 {
			w.body.Write(b[:min(len(b), room)])
		}
	}
	return w.ResponseWriter.Write(b)
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// AuditMiddleware emits audit logs to Kafka.
// It reads/restores the body ONLY for JSON requests on mutating methods.
func AuditMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer *adapters.ProducerAdapter, topic string, opts ...AuditOption) echo.MiddlewareFunc {
	o := auditOptions{
		maxBodyBytes: defaultAuditMaxBodyBytes,
		requestPayload: func(c echo.Context, body []byte) json.RawMessage {
			if json.Valid(body) {
				return json.RawMessage(body)
			}
			logger.Warning(c.Request().Context(), "Invalid JSON in request body")
			return nil
		},
	}
	for _, opt := range opts {
		opt(&o)
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			reqCtx := c.Request().Context()

			var payload json.RawMessage

			// Capture body only for mutating methods AND JSON content
			ct := req.Header.Get(echo.HeaderContentType)
			if method := req.Method; method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {

				if isJSON(ct) && req.Body != nil {
					bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, int64(o.maxBodyBytes)+1))
					truncated := len(bodyBytes) > o.maxBodyBytes
					// Always restore the full body for downstream, even if read failed
					req.Body = struct {
						io.Reader
						io.Closer
					}{io.MultiReader(bytes.NewReader(bodyBytes), req.Body), req.Body}

					switch {
					case err != nil:
						logger.Error(req.Context(), "Failed to read request body: %v", err)
					case truncated:
						logger.Warning(req.Context(), "Request body exceeds %d bytes, omitted from audit", o.maxBodyBytes)
					case len(bodyBytes) > 0:
						payload = o.requestPayload(c, bodyBytes)
					}
				}
			}

			mw := &responseWriter{ResponseWriter: c.Response().Writer, limit: o.maxBodyBytes}
			c.Response().Writer = mw

			callErr := next(c)

			// Capture response status code
			statusCode := c.Response().Status

			// Capture response body as JSON only for error responses (>= 400)
			var responsePayload json.RawMessage
			if statusCode >= 400 && mw.body.Len() > 0 {
				if responseBytes := mw.body.Bytes(); json.Valid(responseBytes) {
					responsePayload = json.RawMessage(responseBytes)
				}
			}

			// Resolve user context
			claims, ok := c.Get("userContext").(*ctx.Context)
			if !ok || claims == nil {
				// If userContext is wrong (any scenario) - eject
				return echo.ErrUnauthorized
			}

			// Parse (or generate) request ID set byt RequestID middleware
			requestID := requestctx.GetOrNewRequestUUID(c.Request().Context())

			// Parse (or generate) session ID set byt RequestID middleware
			sessionID := requestctx.GetOrNewSessionUUID(c.Request().Context())

			tenantID, err := claims.GetTenantId()
			if err != nil {
				logger.Error(c.Request().Context(), "Invalid tenant_id from userContext: %s", claims.Rsc)
				return err
			}

			// Optional message from header
			var message *string
			if val := req.Header.Get("X-Message"); val != "" {
				message = &val
			}

			event := sdkmodels.EventJson{
				Id:          uuid.New(),
				RequestId:   requestID,
				SessionId:   sessionID,
				TenantId:    tenantID,
				EventType:   "audit.log",
				EventSource: utils.CreateServicePrincipleID(cfg),
				Timestamp:   time.Now().UTC(),
				Message:     message,
				Payload: &map[string]any{
					"jti":              claims.Jti,
					"http_method":      req.Method,
					"resource":         deriveResource(c.Path()),
					"endpoint":         c.Path(),
					"full_url":         req.URL.String(),
					"source_ip":        req.RemoteAddr,
					"user_agent":       req.UserAgent(),
					"payload":          payload,
					"subject":          claims.Sub,
					"status_code":      statusCode,
					"response_payload": responsePayload,
				},
			}

			sendEventAsync(reqCtx, producer, logger, topic, event, "audit.log")

			return callErr
		}
	}
}

func deriveResource(path string) string {
	path = strings.Trim(path, "/")
	if parts := strings.Split(path, "/"); len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

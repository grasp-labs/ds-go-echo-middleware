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
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
	ctx "github.com/grasp-labs/ds-go-echo-middleware/middleware/claims"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/requestctx"
)

// isJSON reports whether Content-Type looks like JSON (application/json or */*+json).
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

// AuditMiddleware emits audit logs to Kafka.
// It captures the request body only for JSON content types.
func AuditMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer *adapters.ProducerAdapter, topic string) echo.MiddlewareFunc {
	const maxAuditJSONBytes int64 = 1 << 20 // 1 MiB

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			var payload json.RawMessage
			if m := req.Method; m == http.MethodPost || m == http.MethodPut || m == http.MethodPatch {
				ct := req.Header.Get("Content-Type")
				if isJSON(ct) {
					// Read (capped) JSON body
					limited := io.LimitReader(req.Body, maxAuditJSONBytes+1)
					b, err := io.ReadAll(limited)
					if err != nil {
						logger.Error(req.Context(), "Failed to read request body: %v", err)
					} else {
						if int64(len(b)) > maxAuditJSONBytes {
							logger.Warning(req.Context(), "Request JSON body truncated for audit (> %d bytes)", maxAuditJSONBytes)
							b = b[:maxAuditJSONBytes]
						}
						if json.Valid(b) {
							payload = json.RawMessage(b)
						} else if len(b) > 0 {
							logger.Info(req.Context(), "Request body is not valid JSON (Content-Type: %s); skipping audit payload", ct)
						}
					}
					// ✅ Restore same bytes for downstream handlers
					req.Body = io.NopCloser(bytes.NewReader(b))
				}
				// non-JSON bodies are untouched
			}

			callErr := next(c)

			// Resolve user context
			claims, ok := c.Get("userContext").(*ctx.Context)
			if !ok || claims == nil {
				logger.Info(req.Context(), "Missing or invalid userContext.")
				return WrapErr(c, "unauthorized")
			}

			requestID := requestctx.GetOrNewRequestUUID(c.Request().Context())
			sessionID := requestctx.GetOrNewSessionUUID(c.Request().Context())

			tenantID, err := claims.GetTenantId()
			if err != nil {
				logger.Error(c.Request().Context(), "Invalid tenant_id from userContext: %s", claims.Rsc)
				return err
			}

			event := sdkmodels.EventJson{
				Id:          uuid.New(),
				RequestId:   requestID,
				SessionId:   sessionID,
				TenantId:    tenantID,
				EventType:   "audit.log",
				EventSource: cfg.Name(),
				Timestamp:   time.Now().UTC(),
				Payload: &map[string]any{
					"jti":          claims.Jti,
					"http_method":  req.Method,
					"resource":     deriveResource(c.Path()),
					"endpoint":     c.Path(),
					"full_url":     req.URL.String(),
					"source_ip":    req.RemoteAddr,
					"user_agent":   req.UserAgent(),
					"payload":      payload,
					"subject":      claims.Sub,
					"content_type": req.Header.Get("Content-Type"),
				},
			}

			if kafkaErr := producer.Send(c.Request().Context(), topic, event); kafkaErr != nil {
				logger.Error(c.Request().Context(),
					"Failed to send %s event to Kafka topic '%s' for event ID %s: %v",
					"audit.log", topic, event.Id.String(), kafkaErr)
			}

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

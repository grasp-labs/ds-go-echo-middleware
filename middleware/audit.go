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
	"github.com/grasp-labs/ds-go-echo-middleware/v2/internal/utils"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/adapters"
	ctx "github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/claims"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/requestctx"
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

// AuditMiddleware emits audit logs to Kafka.
// It reads/restores the body ONLY for JSON requests on mutating methods.
func AuditMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer *adapters.ProducerAdapter, topic string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			var payload json.RawMessage

			// Capture body only for mutating methods AND JSON content
			ct := req.Header.Get(echo.HeaderContentType)
			if method := req.Method; method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {

				if isJSON(ct) && req.Body != nil {
					bodyBytes, err := io.ReadAll(req.Body) // read full JSON body
					// Always restore the exact body for downstream, even if read failed
					req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

					if err != nil {
						logger.Error(req.Context(), "Failed to read request body: %v", err)
					} else if len(bodyBytes) > 0 {
						// Validate JSON
						if json.Valid(bodyBytes) {
							payload = json.RawMessage(bodyBytes)
						} else {
							logger.Warning(req.Context(), "Invalid JSON in request body")
						}
					}
				}
			}

			callErr := next(c)

			// Resolve user context
			claims, ok := c.Get("userContext").(*ctx.Context)
			if !ok || claims == nil {
				logger.Info(req.Context(), "Missing or invalid userContext.")
				// Is usercontext is wrong (any scenario) - eject
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
					"jti":         claims.Jti,
					"http_method": req.Method,
					"resource":    deriveResource(c.Path()),
					"endpoint":    c.Path(),
					"full_url":    req.URL.String(),
					"source_ip":   req.RemoteAddr,
					"user_agent":  req.UserAgent(),
					"payload":     payload,
					"subject":     claims.Sub,
				},
			}

			kafkaErr := producer.Send(c.Request().Context(), topic, event)
			if kafkaErr != nil {
				logger.Error(c.Request().Context(), "Failed to send %s event to Kafka topic '%s' for event ID %s: %v", "audit.log", topic, event.Id.String(), kafkaErr)
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

package middleware

import (
	"bytes"
	"encoding/json"
	"io"

	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
	ctx "github.com/grasp-labs/ds-go-echo-middleware/middleware/claims"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/requestctx"
)

// AuditMiddleware returns an Echo middleware that emits audit logs to Kafka.
func AuditMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer *adapters.ProducerAdapter, topic string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			request := c.Request()

			// Capture body only for mutating methods
			var payload json.RawMessage
			if method := request.Method; method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
				bodyBytes, err := io.ReadAll(request.Body)
				if err != nil {
					logger.Error(request.Context(), "Failed to read request body: %v", err)
				} else if len(bodyBytes) > 0 {
					payload = json.RawMessage(bodyBytes)
				}
				// Rewind the body for downstream handlers
				request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}

			callErr := next(c)

			// Resolve user context
			claims, ok := c.Get("userContext").(*ctx.Context)
			if !ok || claims == nil {
				logger.Info(request.Context(), "Missing or invalid userContext.")
				// Is usercontext is wrong (any scenario) - eject
				return WrapErr(c, "uauthorized")
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

			event := sdkmodels.EventJson{
				Id:          requestID,
				SessionId:   sessionID,
				TenantId:    tenantID,
				EventType:   "audit.log",
				EventSource: cfg.Name(),
				Timestamp:   time.Now().UTC(),
				Payload: &map[string]any{
					"jti":         claims.Jti,
					"http_method": request.Method,
					"resource":    deriveResource(c.Path()),
					"endpoint":    c.Path(),
					"full_url":    request.URL.String(),
					"source_ip":   request.RemoteAddr,
					"user_agent":  request.UserAgent(),
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

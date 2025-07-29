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

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/requestctx"
)

// AuditMiddleware returns an Echo middleware that emits audit logs to Kafka.
func AuditMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer interfaces.Producer) echo.MiddlewareFunc {
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

			err := next(c)

			// Resolve user context
			userContext, ok := c.Get("userContext").(*models.Context)
			if !ok || userContext == nil {
				logger.Info(request.Context(), "Missing or invalid userContext.")

				return err
			}

			// Parse (or generate) request ID set byt RequestID middleware
			requestIDStr := requestctx.GetRequestID(c.Request().Context())
			requestID, err := uuid.Parse(requestIDStr)
			if err != nil {
				logger.Error(c.Request().Context(), "Invalid request_id from context: %v", err)
				requestID = uuid.New()
			}

			// Metadata extraction
			entry := models.AuditEntry{
				ID:         requestID,
				TenantID:   userContext.GetTenantId(),
				Subject:    userContext.Sub,
				Jti:        userContext.Jti,
				HTTPMethod: request.Method,
				Resource:   deriveResource(c.Path()),
				Timestamp:  time.Now().UTC(),
				SourceIP:   request.RemoteAddr,
				UserAgent:  request.UserAgent(),
				Service:    cfg.Name(),
				Endpoint:   c.Path(),
				FullURL:    request.URL.String(),
				Payload:    payload,
			}

			kafkaErr := producer.Send(c.Request().Context(), entry.ID.String(), entry)
			if kafkaErr != nil {
				logger.Error(c.Request().Context(), "Failed to send audit entry to Kafka for target %s: %v", entry.ID.String(), err)
			}

			return err
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

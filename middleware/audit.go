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
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/requestctx"
)

// AuditMiddleware returns an Echo middleware that emits audit logs to Kafka.
func AuditMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer *adapters.ProducerAdapter) echo.MiddlewareFunc {
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
			userContext, ok := c.Get("userContext").(*models.Context)
			if !ok || userContext == nil {
				logger.Info(request.Context(), "Missing or invalid userContext.")
				// Is usercontext is wrong (any scenario) - eject
				return WrapErr(c, "uauthorized")
			}

			// Parse (or generate) request ID set byt RequestID middleware
			requestIDStr := requestctx.GetRequestID(c.Request().Context())
			requestID, err := uuid.Parse(requestIDStr)
			if err != nil {
				logger.Error(c.Request().Context(), "Invalid request_id from context: %v", err)
				requestID = uuid.New()
			}

			tenantID, err := userContext.GetTenantId()
			if err != nil {
				logger.Error(c.Request().Context(), "Invalid tenant_id from userContext: %s", userContext.Rsc)
			}

			// Metadata extraction
			auditEvent := models.AuditEntry{
				ID:         requestID,
				TenantID:   tenantID,
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

			auditMap := sdkmodels.EventJson{
				Id:          auditEvent.ID,
				TenantId:    auditEvent.TenantID,
				EventType:   "audit.log",
				EventSource: auditEvent.Service,
				Timestamp:   auditEvent.Timestamp,
				Payload: &map[string]interface{}{
					"jti":         auditEvent.Jti,
					"http_method": auditEvent.HTTPMethod,
					"resource":    auditEvent.Resource,
					"endpoint":    auditEvent.Endpoint,
					"full_url":    auditEvent.FullURL,
					"source_ip":   auditEvent.SourceIP,
					"user_agent":  auditEvent.UserAgent,
					"service":     auditEvent.Service,
					"payload":     auditEvent.Payload,
					"subject":     auditEvent.Subject,
				},
			}

			kafkaErr := producer.Send(c.Request().Context(), auditEvent.ID.String(), auditMap)
			if kafkaErr != nil {
				logger.Error(c.Request().Context(), "Failed to send audit entry to Kafka for target %s: %v", auditEvent.ID.String(), kafkaErr)
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

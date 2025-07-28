package middleware

import (
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/requestctx"
)

// UsageMiddleware returns an Echo middleware that emits usage report to Kafka.
func UsageMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer interfaces.Producer) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			request := c.Request()
			startTimestamp := time.Now().UTC()

			// Call the actual handler
			err := next(c)

			// Retrieve user context
			userContext, ok := c.Get("userContext").(*models.Context)
			if !ok || userContext == nil {
				logger.Error(request.Context(), "Missing or invalid userContext")
				return err
			}

			endTimestamp := time.Now().UTC()

			// Parse (or generate) request ID set byt RequestID middleware
			requestIDStr := requestctx.GetRequestID(c.Request().Context())
			requestID, err := uuid.Parse(requestIDStr)
			if err != nil {
				logger.Error(c.Request().Context(), "invalid request_id from context: %v", err)
				requestID = uuid.New()
			}
			tenantID := userContext.GetTenantId()

			// Optional owner ID from header
			var ownerID *string
			if val := request.Header.Get("X-Owner-ID"); val != "" {
				ownerID = &val
			}

			// Build usage entry
			entry := models.UsageEntry{
				ID:             requestID,
				TenantID:       tenantID,
				OwnerID:        ownerID,
				ProductID:      cfg.ProductID(),
				MemoryMB:       cfg.MemoryLimitMB(),
				StartTimestamp: startTimestamp,
				EndTimestamp:   endTimestamp,
				Status:         models.Draft,
				Metadata: []map[string]string{
					{"user_id": userContext.Sub},
					{"service_name": cfg.Name()},
				},
			}

			kafkaErr := producer.Send(c.Request().Context(), entry.ID.String(), entry)
			if kafkaErr != nil {
				logger.Error(c.Request().Context(), "Failed to send usage entry to Kafka for target %s: %v", entry.ID.String(), kafkaErr)
			}

			return err // return whatever the handler returned
		}
	}
}

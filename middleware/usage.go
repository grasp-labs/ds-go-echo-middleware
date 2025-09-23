package middleware

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/requestctx"
)

// UsageMiddleware returns an Echo middleware that emits usage report to Kafka.
func UsageMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer *adapters.ProducerAdapter) echo.MiddlewareFunc {
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
			tenantID, err := userContext.GetTenantId()
			if err != nil {
				logger.Error(c.Request().Context(), "invalid tenant_id from userContext: %s", userContext.Rsc)
				return err
			}

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
			fmt.Println("ProductID is", entry.ProductID) // DEBUG

			eventMap := sdkmodels.EventJson{
				Id:        entry.ID,
				TenantId:  entry.TenantID,
				OwnerId:   entry.OwnerID,
				Timestamp: entry.StartTimestamp,
				Payload: &map[string]interface{}{
					"product_id":   entry.ProductID,
					"memory_mb":    entry.MemoryMB,
					"start_time":   entry.StartTimestamp,
					"end_time":     entry.EndTimestamp,
					"status":       entry.Status,
					"user_id":      userContext.Sub, // safer and clearer
					"service_name": cfg.Name(),      // safer and clearer
				},
			}

			kafkaErr := producer.Send(c.Request().Context(), entry.ID.String(), eventMap)
			if kafkaErr != nil {
				logger.Error(c.Request().Context(), "Failed to send usage entry to Kafka for target %s: %v", entry.ID.String(), kafkaErr)
			}

			return err // return whatever the handler returned
		}
	}
}

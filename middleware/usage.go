package middleware

import (
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
	ctx "github.com/grasp-labs/ds-go-echo-middleware/middleware/claims"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/requestctx"
)

// UsageMiddleware returns an Echo middleware that emits usage report to Kafka.
func UsageMiddleware(cfg interfaces.Config, logger interfaces.Logger, producer *adapters.ProducerAdapter, topic string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			request := c.Request()
			startTimestamp := time.Now().UTC()

			// Call the actual handler
			callErr := next(c)

			// Retrieve user context
			claims, ok := c.Get("userContext").(*ctx.Context)
			if !ok || claims == nil {
				logger.Error(request.Context(), "Missing or invalid userContext")
				// Is usercontext is wrong (any scenario) - eject
				return WrapErr(c, "uauthorized")
			}

			endTimestamp := time.Now().UTC()

			// Parse (or generate) request ID set byt RequestID middleware
			requestID := requestctx.GetOrNewRequestUUID(c.Request().Context())
			sessionID := requestctx.GetOrNewSessionUUID(c.Request().Context())

			tenantID, err := claims.GetTenantId()
			if err != nil {
				logger.Error(c.Request().Context(), "invalid tenant_id from userContext: %s", claims.Rsc)
				return err
			}

			// Optional owner ID from header
			var ownerID *string
			if val := request.Header.Get("X-Owner-ID"); val != "" {
				ownerID = &val
			}

			event := sdkmodels.EventJson{
				Id:        uuid.New(),
				RequestId: requestID,
				SessionId: sessionID,
				TenantId:  tenantID,
				OwnerId:   ownerID,
				Timestamp: startTimestamp,
				Payload: &map[string]any{
					"product_id":   cfg.ProductID(),
					"memory_mb":    cfg.MemoryLimitMB(),
					"start_time":   startTimestamp,
					"end_time":     endTimestamp,
					"status":       models.Draft,
					"user_id":      claims.Sub,
					"service_name": cfg.Name(),
				},
			}

			kafkaErr := producer.Send(c.Request().Context(), topic, event)
			if kafkaErr != nil {
				logger.Error(c.Request().Context(), "Failed to send usage event to Kafka topic '%s' for event ID %s: %v", topic, event.Id.String(), kafkaErr)
			}

			return callErr
		}
	}
}

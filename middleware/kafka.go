package middleware

import (
	"context"
	"time"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/interfaces"
)

// sendEventAsync fires a Kafka event in a detached goroutine so it never
// blocks the HTTP response. A 5-second timeout ensures the goroutine exits
// cleanly if Kafka is unavailable.
func sendEventAsync(producer *adapters.ProducerAdapter, logger interfaces.Logger, topic string, event sdkmodels.EventJson, eventType string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := producer.Send(ctx, topic, event); err != nil {
			logger.Error(ctx, "Failed to send %s event: %v", eventType, err)
		}
	}()
}

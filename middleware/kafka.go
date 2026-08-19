package middleware

import (
	"context"
	"time"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/adapters"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/interfaces"
)

const kafkaSendTimeout = 3 * time.Second

// sendEventAsync fires a Kafka event in a detached goroutine so it never
// blocks the HTTP response. It detaches from the request's cancellation
// (via context.WithoutCancel) while retaining request-scoped values, and
// bounds the send with kafkaSendTimeout so an unavailable broker can't
// leak goroutines.
func sendEventAsync(reqCtx context.Context, producer *adapters.ProducerAdapter, logger interfaces.Logger, topic string, event sdkmodels.EventJson, eventType string) {
	base := context.WithoutCancel(reqCtx)
	go func() {
		ctx, cancel := context.WithTimeout(base, kafkaSendTimeout)
		defer cancel()
		if err := producer.Send(ctx, topic, event); err != nil {
			logger.Error(ctx, "Failed to send %s event: %v", eventType, err)
		}
	}()
}

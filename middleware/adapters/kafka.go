package adapters

import (
	"context"
	"fmt"

	sdkKafka "github.com/grasp-labs/ds-event-stream-go-sdk/kafka"
	sdkModels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/interfaces"
)

type Producer interface {
	Send(ctx context.Context, key string, value any) error
	Close() error
}

type ProducerAdapter struct {
	Producer interfaces.Producer
}

func (a *ProducerAdapter) Send(ctx context.Context, key string, value any) error {
	return a.Producer.Send(ctx, key, value)
}

func (a *ProducerAdapter) Close() error {
	return a.Producer.Close()
}

// KafkaProducerWrapper implements interfaces.Producer for the real Kafka producer
type KafkaProducerWrapper struct {
	Producer *sdkKafka.Producer
}

func (w *KafkaProducerWrapper) Send(ctx context.Context, key string, value any) error {
	event, ok := value.(sdkModels.EventJson)
	if !ok {
		return fmt.Errorf("KafkaProducerWrapper: value is not models.EventJson")
	}
	return w.Producer.SendEvent(ctx, key, event)
}

func (w *KafkaProducerWrapper) Close() error {
	return w.Producer.Close()
}

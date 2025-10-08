package adapters

import (
	"context"
	"fmt"

	dskafka "github.com/grasp-labs/ds-event-stream-go-sdk/dskafka"
	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/interfaces"
)

type ProducerAdapter struct {
	Producer interfaces.Producer
}

func (a *ProducerAdapter) Send(ctx context.Context, topic string, value any) error {
	return a.Producer.Send(ctx, topic, value)
}

func (a *ProducerAdapter) Close() error {
	return a.Producer.Close()
}

// KafkaProducerWrapper implements interfaces.Producer for the real Kafka producer
type KafkaProducerWrapper struct {
	Producer *dskafka.Producer
}

func (w *KafkaProducerWrapper) Send(ctx context.Context, topic string, value any) error {
	event, ok := value.(sdkmodels.EventJson)
	if !ok {
		return fmt.Errorf("KafkaProducerWrapper: value is not sdkmodels.EventJson")
	}
	return w.Producer.SendEvent(ctx, topic, event)
}

func (w *KafkaProducerWrapper) Close() error {
	return w.Producer.Close()
}

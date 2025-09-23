package adapters

import (
	"context"
	"errors"

	"github.com/grasp-labs/ds-event-stream-go-sdk/models"
)

type ProducerAdapter struct {
	Producer interface {
		SendEvent(ctx context.Context, key string, value any) error
		Close() error
	}
}

func (a *ProducerAdapter) Send(ctx context.Context, key string, value any) error {
	event, ok := value.(models.EventJson)
	if !ok {
		return errors.New("ProducerAdapter: value is not models.EventJson")
	}
	return a.Producer.SendEvent(ctx, key, event)
}

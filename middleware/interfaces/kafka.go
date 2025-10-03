package interfaces

import "context"

type Producer interface {
	Send(ctx context.Context, topic string, value any) error
	Close() error
}

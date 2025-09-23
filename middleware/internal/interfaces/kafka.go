package interfaces

import "context"

type Producer interface {
	SendEvent(ctx context.Context, key string, value any) error
	Close() error
}

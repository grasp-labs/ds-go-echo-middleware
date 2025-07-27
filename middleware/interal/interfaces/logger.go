package interfaces

import "context"

type Logger interface {
	Info(ctx context.Context, msg string)
}

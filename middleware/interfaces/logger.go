package interfaces

import "context"

type Logger interface {
	Info(ctx context.Context, format string, args ...any)
	Warning(ctx context.Context, format string, args ...any)
	Error(ctx context.Context, format string, args ...any)
}

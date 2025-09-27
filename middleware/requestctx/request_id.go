package requestctx

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// GetRequestID returns the request ID from context.
func GetRequestID(ctx context.Context) string {
	val, _ := ctx.Value(requestIDKey).(string)
	return val
}

// SetRequestID sets the request ID in the context.
func SetRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func GetOrNewRequestUUID(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		if u, err := uuid.Parse(v); err == nil {
			return u
		}
	}
	return uuid.New()
}

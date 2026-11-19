package requestctx

import (
	"context"

	"github.com/google/uuid"
)

const sessionIDKey ctxKey = "session_id"

// GetSessionID returns the request ID from context.
func GetSessionID(ctx context.Context) string {
	val, _ := ctx.Value(sessionIDKey).(string)
	return val
}

// SetSessionID sets the request ID in the context.
func SetSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey, id)
}

// GetOrNewSessionUUID returns the session ID as uuid.UUID, generating a new one if missing/invalid.
func GetOrNewSessionUUID(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(sessionIDKey).(string); ok {
		if u, err := uuid.Parse(v); err == nil {
			return u
		}
	}
	return uuid.New()
}

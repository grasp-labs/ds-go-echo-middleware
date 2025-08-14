package requestctx

import (
	"context"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/claims"
)

const userContextKey ctxKey = "userContext"

// GetUserContext returns the Context model from context.
func GetUserContext(ctx context.Context) *claims.Context {
	val := ctx.Value(userContextKey).(*claims.Context)
	return val
}

// SetRequestID sets the request ID in the context.
func SetUserContext(ctx context.Context, userContext *claims.Context) context.Context {
	return context.WithValue(ctx, userContextKey, userContext)
}

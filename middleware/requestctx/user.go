package requestctx

import (
	"context"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
)

const userContextKey ctxKey = "userContext"

// GetUserContext returns the Context model from context.
func GetUserContext(ctx context.Context) *models.Context {
	val := ctx.Value(userContextKey).(*models.Context)
	return val
}

// SetRequestID sets the request ID in the context.
func SetUserContext(ctx context.Context, userContext *models.Context) context.Context {
	return context.WithValue(ctx, userContextKey, userContext)
}

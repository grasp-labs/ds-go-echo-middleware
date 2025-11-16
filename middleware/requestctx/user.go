package requestctx

import (
	"context"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/claims"
)

var userContextKey ctxKey = "userContext"

func GetUserContext(ctx context.Context) *claims.Context {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(userContextKey)
	uc, ok := v.(*claims.Context)
	if ok && uc != nil {
		return uc
	}

	if v := ctx.Value(userContextKey); v != nil {
		if uc, ok := v.(*claims.Context); ok && uc != nil {
			return uc
		}
	}
	return nil
}

func SetUserContext(ctx context.Context, user *claims.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, userContextKey, user)
}

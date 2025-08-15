// middleware/requestctx/user.go
package requestctx

import (
	"context"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/claims"
)

var userContextKey ctxKey

func GetUserContext(ctx context.Context) *claims.Context {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(userContextKey)
	uc, ok := v.(*claims.Context)
	if ok && uc != nil {
		return uc
	}
	// (optional legacy fallback)
	if v := ctx.Value("userContext"); v != nil {
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

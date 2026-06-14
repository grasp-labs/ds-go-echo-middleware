package requestctx

import (
	"context"

	"github.com/google/uuid"

	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/claims"
)

// Principal kinds, derived from the token's `cls` claim.
const (
	KindUser = "user" // sub is a human's email
	KindApp  = "app"  // sub is an app's client_id
)

// Principal is the normalized identity extracted from a verified token. It is
// the same shape for both user and app principals; branch on Kind only for
// human-only / machine-only routes.
type Principal struct {
	Kind     string    // "user" | "app"
	ID       string    // sub: email (user) or client_id (app)
	TenantID uuid.UUID // parsed from rsc (substring before the first ':')
	Roles    []string  // rol: coarse flags, advisory only
	JTI      uuid.UUID // token id, for audit
}

// ValidKind reports whether cls is a recognized principal kind.
func ValidKind(cls string) bool {
	return cls == KindUser || cls == KindApp
}

// NewPrincipal builds a Principal from verified claims. The tenant id is parsed
// from rsc; an error there is returned so the caller can reject the token.
func NewPrincipal(c *claims.Context) (Principal, error) {
	tenantID, err := c.GetTenantId()
	if err != nil {
		return Principal{}, err
	}
	return Principal{
		Kind:     c.Cls,
		ID:       c.Sub,
		TenantID: tenantID,
		Roles:    c.Rol,
		JTI:      c.Jti,
	}, nil
}

var principalKey ctxKey = "principal"

// GetPrincipal returns the normalized principal from context, if present.
func GetPrincipal(ctx context.Context) (Principal, bool) {
	if ctx == nil {
		return Principal{}, false
	}
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

// SetPrincipal stores the normalized principal in the context.
func SetPrincipal(ctx context.Context, p Principal) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, principalKey, p)
}

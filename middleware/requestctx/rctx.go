package requestctx

import (
	"context"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/claims"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/interfaces"
)

// RequestCtx is a struct that holds information about the current request context.
// It includes the original context, the request ID,
// the locale, the user claims, the tenant ID,
// and any error that occurred during processing.
type RequestCtx struct {
	Ctx       context.Context
	C         echo.Context
	RequestID uuid.UUID
	Locale    string
	Claims    *claims.Context
	TenantID  uuid.UUID
	Err       error
}

// Deprecated: Use New() instead. This method is kept for backward compatibility and will be removed in future versions.
// New creates a new RequestCtx from the given Echo context and configuration.
func (r RequestCtx) New(c echo.Context, cfg interfaces.Config) RequestCtx {
	return New(c, cfg)
}

// New creates a new RequestCtx from the given Echo context and configuration.
//
// It extracts the user claims, tenant ID, and request ID from the request context,
// and determines the locale from the Echo context (if set) or falls back to the
// default language provided by the configuration.
func New(c echo.Context, cfg interfaces.Config) RequestCtx {
	ctx := c.Request().Context()
	ctxClaims := GetUserContext(ctx)

	// Extract tenant ID from claims if available, otherwise set to uuid.Nil
	var tenantID = uuid.Nil
	var err error
	if ctxClaims != nil {
		tenantID, err = ctxClaims.GetTenantId()

		if err != nil {
			tenantID = uuid.Nil
		}
	}

	requestID := GetOrNewRequestUUID(ctx)

	// Get locale from echo context (set by LocaleMiddleware for this request)
	// or fallback to config's Language() method if implemented, or "en" as final default
	locale := "en"
	if langProvider, ok := cfg.(interface{ Language() string }); ok {
		locale = langProvider.Language()
	}
	if v, ok := c.Get("locale").(string); ok && v != "" {
		locale = v
	}

	return RequestCtx{
		Ctx:       ctx,
		C:         c,
		RequestID: requestID,
		Locale:    locale,
		Claims:    ctxClaims,
		TenantID:  tenantID,
		Err:       err,
	}
}

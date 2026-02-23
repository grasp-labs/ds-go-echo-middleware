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

// New creates a new RequestCtx from the given context and locale.
//
//	It extracts the user claims, tenant ID, and request ID from the context.
//
// If any of these values are missing or invalid,
// it sets them to default values (e.g., uuid.Nil for tenant ID and request ID).
func (r *RequestCtx) New(c echo.Context, cfg interfaces.Config) RequestCtx {
	ctx := c.Request().Context()
	claims := GetUserContext(ctx)

	// Extract tenant ID from claims if available, otherwise set to uuid.Nil
	var tenantID = uuid.Nil
	if claims != nil {
		var err error
		tenantID, err = claims.GetTenantId()

		if err != nil {
			tenantID = uuid.Nil
		}
	}

	requestID, err := uuid.Parse(GetRequestID(ctx))
	if err != nil {
		requestID = uuid.New()
	}

	// Get locale from echo context (set by LocaleMiddleware) or fallback to default from config
	locale := cfg.Language()
	if v := c.Get("locale"); v != nil {
		if s, ok := v.(string); ok && s != "" {
			locale = s
		}
	}

	return RequestCtx{
		Ctx:       ctx,
		C:         c,
		RequestID: requestID,
		Locale:    locale,
		Claims:    claims,
		TenantID:  tenantID,
		Err:       err,
	}
}

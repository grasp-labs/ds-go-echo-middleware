package requestctx

import (
	"context"

	"github.com/google/uuid"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/claims"
)

// RequestCtx is a struct that holds information about the current request context.
// It includes the original context, the request ID,
// the locale, the user claims, the tenant ID,
// and any error that occurred during processing.
type RequestCtx struct {
	Ctx       context.Context
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
func New(c context.Context, locale string) RequestCtx {
	claims := GetUserContext(c)

	var tenantID = uuid.Nil

	if claims != nil {
		var err error
		tenantID, err = claims.GetTenantId()

		if err != nil {
			tenantID = uuid.Nil
		}
	}

	requestID, err := uuid.Parse(GetRequestID(c))
	if err != nil {
		requestID = uuid.New()
	}

	return RequestCtx{
		Ctx:       c,
		Locale:    locale,
		Claims:    claims,
		TenantID:  tenantID,
		RequestID: requestID,
		Err:       err,
	}
}

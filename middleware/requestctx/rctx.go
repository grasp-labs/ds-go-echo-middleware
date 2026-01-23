package requestctx

import (
	"context"

	"github.com/google/uuid"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/claims"
)

type RequestCtx struct {
	Ctx       context.Context
	RequestID uuid.UUID
	Locale    string
	Claims    *claims.Context
	TenantID  uuid.UUID
	Err       error
}

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

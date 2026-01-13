package requestctx

import (
	"context"

	"github.com/google/uuid"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/claims"

	"github.com/labstack/echo/v4"
)

type RequestCtx struct {
	Ctx       context.Context
	C         echo.Context
	RequestID uuid.UUID
	Locale    string
	Claims    *claims.Context
	TenantID  uuid.UUID
	Err       error
}

func (r *RequestCtx) New(c echo.Context, locale string) RequestCtx {
	ctx := c.Request().Context()
	claims := GetUserContext(ctx)

	var tenantID uuid.UUID
	var err error
	if claims != nil {
		tenantID, err = claims.GetTenantId()
		if err != nil {
			tenantID = uuid.Nil
		}
	} else {
		tenantID = uuid.Nil
	}

	requestID, err := uuid.Parse(GetRequestID(ctx))
	if err != nil {
		requestID = uuid.New()
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

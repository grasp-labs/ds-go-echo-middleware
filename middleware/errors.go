package middleware

import (
	"github.com/labstack/echo/v4"

	errCode "github.com/grasp-labs/ds-go-commonmodels/v3/commonmodels/enum/errors"
	httpErr "github.com/grasp-labs/ds-go-commonmodels/v3/commonmodels/http_error"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/requestctx"
)

const DefaultLocal string = "en"

// Resolve builds a localized HTTP error from a machine code.
// Usage: status, body := Resolve(c, errCode.InvalidEmailFormat, "email"); return c.JSON(status, body)
func ResolveErr(c echo.Context, machineCode string, args ...any) (int, *httpErr.HTTPError) {
	locale := Locale(c, "en")                                       // fall back to "en"
	status := errCode.StatusFor(machineCode)                        // code -> HTTP status
	msg := errCode.HumanMessageLocale(locale, machineCode, args...) // fill %s etc.
	requestID := requestctx.GetRequestID(c.Request().Context())
	he := httpErr.NewHTTPError(requestID, machineCode, msg, status)
	return status, he
}

// Wrap builds a localized HTTP error from a machine code.
// Usage: err := Wrap(c, errCode.InvalidEmailFormat, "email")
func WrapErr(c echo.Context, machineCode string, args ...any) *httpErr.HTTPError {
	locale := Locale(c, DefaultLocal)
	msg := errCode.HumanMessageLocale(locale, machineCode, args...)
	requestID := requestctx.GetRequestID(c.Request().Context())
	return httpErr.NewHTTPError(requestID, machineCode, msg, errCode.StatusFor(machineCode))
}

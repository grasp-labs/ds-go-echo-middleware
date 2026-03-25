package middleware

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/interfaces"
)

const defaultAPIKeyHeader = "X-Api-Key"

// APIKeyMiddleware returns middleware that validates the request API key against
// one of validKeys using the X-Api-Key header. CORS preflight (OPTIONS) is skipped.
func APIKeyMiddleware(logger interfaces.Logger, validKeys []string) (echo.MiddlewareFunc, error) {
	if len(validKeys) == 0 {
		return nil, errors.New("api key middleware: at least one valid key is required")
	}
	keys := append([]string(nil), validKeys...)

	return middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		KeyLookup:  "header:" + defaultAPIKeyHeader,
		AuthScheme: "",
		Skipper: func(c echo.Context) bool {
			return c.Request().Method == http.MethodOptions
		},
		Validator: func(key string, c echo.Context) (bool, error) {
			if key == "" {
				logger.Error(c.Request().Context(), "API key is empty")
				return false, WrapErr(c, "unauthorized")
			}
			for _, valid := range keys {
				if constantTimeStringEqual(key, valid) {
					return true, nil
				}
			}
			logger.Error(c.Request().Context(), "Invalid API key")
			return false, WrapErr(c, "unauthorized")
		},
		ErrorHandler: func(handlerErr error, c echo.Context) error {
			logger.Error(c.Request().Context(), "API key error: %v", handlerErr)
			return echo.ErrUnauthorized
		},
	}), nil
}

func constantTimeStringEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

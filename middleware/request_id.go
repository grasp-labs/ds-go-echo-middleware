package middleware

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/requestctx"
)

const (
	headerRequestID = "X-Request-ID"
	headerSessionID = "X-Session-ID"
)

// RequestIDMiddleware propagates valid request and session UUIDs in canonical
// lowercase form, generating UUIDs for missing or invalid values.
func RequestIDMiddleware(logger interfaces.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()

			requestID := normalizeOrNewUUID(req.Header.Get(headerRequestID))
			sessionID := normalizeOrNewUUID(req.Header.Get(headerSessionID))

			ctx := requestctx.SetRequestID(req.Context(), requestID)
			if sessionID != "" {
				ctx = requestctx.SetSessionID(ctx, sessionID)
			}
			c.SetRequest(req.WithContext(ctx))

			res.Header().Set(headerRequestID, requestID)
			if sessionID != "" {
				res.Header().Set(headerSessionID, sessionID)
			}

			return next(c)
		}
	}
}

func normalizeOrNewUUID(value string) string {
	id, err := uuid.Parse(value)
	// Validate checks the wrapper characters that Parse skips.
	if err != nil || uuid.Validate(value) != nil {
		return uuid.NewString()
	}
	// Use the same lowercase, hyphenated ID in headers and logs for every accepted spelling.
	return id.String()
}

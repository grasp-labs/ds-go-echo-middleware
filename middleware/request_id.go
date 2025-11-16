package middleware

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/requestctx"
)

const (
	headerRequestID = "X-Request-ID"
	headerSessionID = "X-Session-ID"
)

// RequestIDMiddleware sets X-Request-ID if missing, and propagates it.
// SessionID is propagated if valid; if invalid/missing, we leave it unset (adjust if you want to synthesize one).
func RequestIDMiddleware(logger interfaces.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()

			// Request ID: ensure valid UUID
			requestID := req.Header.Get(headerRequestID)
			if _, err := uuid.Parse(requestID); err != nil || requestID == "" {
				requestID = uuid.New().String()
			}

			// Session ID: propagate always
			sessionID := req.Header.Get(headerSessionID)
			if _, err := uuid.Parse(sessionID); err != nil || sessionID == "" {
				sessionID = uuid.New().String()
			}

			// Put IDs into context (preserve previously set values)
			ctx := requestctx.SetRequestID(req.Context(), requestID)
			if sessionID != "" {
				ctx = requestctx.SetSessionID(ctx, sessionID)
			}
			c.SetRequest(req.WithContext(ctx))

			// Expose via response headers
			res.Header().Set(headerRequestID, requestID)
			if sessionID != "" {
				res.Header().Set(headerSessionID, sessionID)
			}

			return next(c)
		}
	}
}

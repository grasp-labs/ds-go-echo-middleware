package middleware

import (
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/interfaces"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/requestctx"
)

// RequestIDMiddleware sets X-Request-ID if missing, and propagates it
func RequestIDMiddleware(logger interfaces.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()

			requestID := req.Header.Get("X-Request-ID")

			// Validate RequestID
			_, err := uuid.Parse(requestID)
			if err != nil {
				requestID = uuid.New().String()
			}

			// Set in request and response headers
			req.Header.Set("X-Request-ID", requestID)
			res.Header().Set("X-Request-ID", requestID)

			ctx := requestctx.SetRequestID(req.Context(), requestID)

			c.SetRequest(req.WithContext(ctx))

			res.Header().Set("X-Request-ID", requestID)

			logger.Info(c.Request().Context(), "Request started")

			return next(c)
		}
	}
}

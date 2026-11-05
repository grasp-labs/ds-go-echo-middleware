package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// RequestLogger logs one completion event after the response status is known.
// Register it before Recover and middleware that may return early.
// Use a logctx handler to include request and principal fields.
func RequestLogger(logger *slog.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		HandleError:      true,
		LogLatency:       true,
		LogProtocol:      true,
		LogRemoteIP:      true,
		LogMethod:        true,
		LogURI:           true,
		LogRoutePath:     true,
		LogRequestID:     false, // The context handler supplies the canonical ID.
		LogUserAgent:     true,
		LogStatus:        true,
		LogError:         true,
		LogContentLength: true,
		LogResponseSize:  true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			level := slog.LevelInfo
			if v.Status >= http.StatusInternalServerError {
				level = slog.LevelError
			}

			attrs := []slog.Attr{
				slog.String("method", v.Method),
				slog.String("uri", v.URI),
				slog.Int("status", v.Status),
				slog.Float64("duration_ms", float64(v.Latency)/float64(time.Millisecond)),
				slog.String("protocol", v.Protocol),
				slog.String("remote_ip", v.RemoteIP),
				slog.String("user_agent", v.UserAgent),
				slog.String("bytes_in", v.ContentLength),
				slog.Int64("bytes_out", v.ResponseSize),
			}
			if v.RoutePath != "" {
				attrs = append(attrs, slog.String("route", v.RoutePath))
			}
			if v.Error != nil {
				attrs = append(attrs, slog.Any("error", v.Error))
			}
			// Read the updated context after downstream middleware adds identity.
			logger.LogAttrs(c.Request().Context(), level, "request completed", attrs...)
			return nil
		},
	})
}

// Recover logs panics and returns errors to the outer RequestLogger.
func Recover(logger *slog.Logger) echo.MiddlewareFunc {
	return middleware.RecoverWithConfig(middleware.RecoverConfig{
		DisableStackAll:     true,
		DisableErrorHandler: true,
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			logger.ErrorContext(c.Request().Context(), "request panicked", "error", err, "stack", string(stack))
			return err
		},
	})
}

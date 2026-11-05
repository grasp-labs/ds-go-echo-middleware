package middleware_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/internal/testlog"
	mw "github.com/grasp-labs/ds-go-echo-middleware/v3/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/logctx"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/stretchr/testify/require"
)

func TestRequestLoggerStatus(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		err          error
		missingRoute bool
		level        string
	}{
		{name: "success", status: http.StatusOK, level: "INFO"},
		{name: "client error", status: http.StatusBadRequest, err: echo.ErrBadRequest, level: "INFO"},
		{name: "written overload", status: http.StatusTooManyRequests, level: "INFO"},
		{name: "not found", status: http.StatusNotFound, missingRoute: true, level: "INFO"},
		{name: "handler failure", status: http.StatusInternalServerError, err: errors.New("handler failed"), level: "ERROR"},
		{name: "written server error", status: http.StatusInternalServerError, level: "ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			e := echo.New()
			e.Use(mw.RequestLogger(logctx.New(&output)))
			if !tc.missingRoute {
				e.GET("/public", func(c echo.Context) error {
					if tc.err != nil {
						return tc.err
					}
					return c.NoContent(tc.status)
				})
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/public", nil)
			req.Header.Set(echo.HeaderXRequestID, "untrusted-header")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, tc.status, rec.Code)
			entries := testlog.Read(t, &output)
			require.Len(t, entries, 1)
			entry := entries[0]
			require.Equal(t, "request completed", entry["msg"])
			require.Equal(t, float64(tc.status), entry["status"])
			require.Equal(t, tc.level, entry["level"])
			require.NotContains(t, entry, "request_id", "request IDs must come from context")
			switch {
			case tc.err != nil:
				require.Equal(t, tc.err.Error(), entry["error"])
			case tc.missingRoute:
				require.Equal(t, echo.ErrNotFound.Error(), entry["error"])
				require.NotContains(t, entry, "route")
			default:
				require.NotContains(t, entry, "error")
			}
		})
	}
}

func TestRequestLoggerUsesFinalStatus(t *testing.T) {
	var output bytes.Buffer
	e := echo.New()
	e.Use(mw.RequestLogger(logctx.New(&output)))
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if !c.Response().Committed {
			require.NoError(t, c.NoContent(http.StatusServiceUnavailable))
		}
	}
	e.GET("/", func(echo.Context) error { return echo.ErrBadRequest })
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	entries := testlog.Read(t, &output)
	require.Len(t, entries, 1)
	require.Equal(t, float64(http.StatusServiceUnavailable), entries[0]["status"])
	require.Equal(t, "ERROR", entries[0]["level"])
	require.Equal(t, echo.ErrBadRequest.Error(), entries[0]["error"])
}

func TestRequestLoggerUsesCanonicalRequestContext(t *testing.T) {
	const responseBody = "item ready ✓"
	var output bytes.Buffer
	logger := logctx.New(&output)
	e := echo.New()
	e.Use(mw.RequestLogger(logger), mw.Recover(logger))
	e.POST("/items/:id", func(c echo.Context) error {
		logger.InfoContext(c.Request().Context(), "handler log")
		return c.String(http.StatusOK, responseBody)
	}, mw.RequestIDMiddleware(logctx.Adapter{Logger: logger}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/items/42?status=active&status=closed&key=folder%2Fitem", bytes.NewBufferString("request body"))
	req.RemoteAddr = "192.0.2.10:4321"
	req.Header.Set("User-Agent", "ds-state-test/1.0")
	req.Header.Set(echo.HeaderContentLength, strconv.FormatInt(req.ContentLength, 10))
	req.Header.Set(echo.HeaderXRequestID, "invalid-request-id")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, responseBody, rec.Body.String())
	requestID := rec.Header().Get(echo.HeaderXRequestID)
	require.NoError(t, uuid.Validate(requestID))
	entries := testlog.Read(t, &output)
	require.Len(t, entries, 2)
	require.Contains(t, entries[1], "duration_ms")
	entries[1]["duration_ms"] = "<duration>"
	for _, entry := range entries {
		require.Equal(t, requestID, entry["request_id"])
		entry["request_id"] = "<request-id>"
	}
	testlog.Equal(t, `[
		{
			"level": "INFO",
			"msg": "handler log",
			"request_id": "<request-id>"
		},
		{
			"level": "INFO",
			"msg": "request completed",
			"request_id": "<request-id>",
			"method": "POST",
			"uri": "/items/42?status=active&status=closed&key=folder%2Fitem",
			"route": "/items/:id",
			"status": 200,
			"protocol": "HTTP/1.1",
			"remote_ip": "192.0.2.10",
			"user_agent": "ds-state-test/1.0",
			"duration_ms": "<duration>",
			"bytes_in": "12",
			"bytes_out": 14
		}
	]`, entries)
}

func TestRequestLoggerDuration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var output bytes.Buffer
		e := echo.New()
		e.Use(mw.RequestLogger(logctx.New(&output)))
		e.GET("/", func(c echo.Context) error {
			time.Sleep(25 * time.Millisecond)
			return c.NoContent(http.StatusOK)
		})
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
		entries := testlog.Read(t, &output)
		require.Len(t, entries, 1)
		require.Equal(t, float64(25), entries[0]["duration_ms"])
	})
}

func TestRequestLoggerIncludesPreflight(t *testing.T) {
	var output bytes.Buffer
	e := echo.New()
	e.Use(mw.RequestLogger(logctx.New(&output)), echomw.CORS())
	e.GET("/", func(echo.Context) error {
		t.Fatal("preflight must not reach the handler")
		return nil
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/", nil)
	req.Header.Set(echo.HeaderOrigin, "https://grasp-daas.com")
	req.Header.Set(echo.HeaderAccessControlRequestMethod, http.MethodGet)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	entries := testlog.Read(t, &output)
	require.Len(t, entries, 1)
	require.Equal(t, float64(http.StatusNoContent), entries[0]["status"])
	require.Equal(t, http.MethodOptions, entries[0]["method"])
}

func TestRecoverLogsPanicAndCompletion(t *testing.T) {
	var output bytes.Buffer
	logger := logctx.New(&output)
	e := echo.New()
	e.Use(mw.RequestLogger(logger), mw.Recover(logger))
	e.GET("/", func(echo.Context) error { panic("handler panic") }, mw.RequestIDMiddleware(logctx.Adapter{Logger: logger}))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	entries := testlog.Read(t, &output)
	require.Len(t, entries, 2)
	require.Equal(t, "request panicked", entries[0]["msg"])
	require.NotEmpty(t, entries[0]["stack"])
	require.Equal(t, "request completed", entries[1]["msg"])
	require.Equal(t, float64(http.StatusInternalServerError), entries[1]["status"])
	requestID := rec.Header().Get(echo.HeaderXRequestID)
	require.NotEmpty(t, requestID)
	for _, entry := range entries {
		require.Equal(t, "ERROR", entry["level"])
		require.Equal(t, "handler panic", entry["error"])
		require.Equal(t, requestID, entry["request_id"])
	}
}

func TestRecoverPreservesAbortHandler(t *testing.T) {
	var output bytes.Buffer
	logger := logctx.New(&output)
	e := echo.New()
	e.Use(mw.RequestLogger(logger), mw.Recover(logger))
	e.GET("/", func(echo.Context) error { panic(http.ErrAbortHandler) })
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	})
	require.Empty(t, output.String())
}

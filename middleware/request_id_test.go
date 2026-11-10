package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/grasp-labs/ds-go-echo-middleware/v3/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/requestctx"
)

func TestRequestIDMiddlewareNormalizesUUIDs(t *testing.T) {
	const canonical = "89b9c527-e093-4b1d-99d7-04abc5de73dd"
	for _, tc := range []struct {
		input string
		want  string // Empty when the middleware should generate a UUIDv4.
	}{
		{canonical, canonical},
		{"89B9C527-E093-4B1D-99D7-04ABC5DE73DD", canonical},
		{"89B9C527E0934B1D99D704ABC5DE73DD", canonical},
		{"{89B9C527-E093-4B1D-99D7-04ABC5DE73DD}", canonical},
		{"urn:uuid:89B9C527-E093-4B1D-99D7-04ABC5DE73DD", canonical},
		{uuid.Nil.String(), uuid.Nil.String()},
		{"!00000000-0000-0000-0000-000000000000!", ""},
		{"invalid", ""},
		{"", ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			e := echo.New()
			e.Use(middleware.RequestIDMiddleware(&fakes.MockLogger{}))
			var requestID, sessionID string
			e.GET("/", func(c echo.Context) error {
				requestID = requestctx.GetRequestID(c.Request().Context())
				sessionID = requestctx.GetSessionID(c.Request().Context())
				return c.NoContent(http.StatusOK)
			})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			req.Header.Set(echo.HeaderXRequestID, tc.input)
			req.Header.Set("X-Session-ID", tc.input)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
			for header, value := range map[string]string{
				echo.HeaderXRequestID: requestID,
				"X-Session-ID":        sessionID,
			} {
				parsed, err := uuid.Parse(value)
				require.NoError(t, err)
				require.Equal(t, parsed.String(), value)
				require.Equal(t, value, rec.Header().Get(header))
				if tc.want == "" {
					require.Equal(t, uuid.Version(4), parsed.Version(), header)
				} else {
					require.Equal(t, tc.want, value, header)
				}
			}
		})
	}
}

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/requestctx"
)

func TestRequestIDMiddleware_GeneratesID(t *testing.T) {
	e := echo.New()
	logger := &mockLogger{}

	e.Use(middleware.RequestIDMiddleware(logger))

	var capturedRequestID string

	e.GET("/", func(c echo.Context) error {
		// extract the ID from context
		id := requestctx.GetRequestID(c.Request().Context())
		capturedRequestID = id
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	// Assert response has a valid UUID
	headerID := res.Header.Get("X-Request-ID")
	_, err := uuid.Parse(headerID)
	assert.NoError(t, err)
	assert.Equal(t, headerID, capturedRequestID)

	// Logger should be called
	assert.True(t, logger.infoCalled)
	assert.True(t, strings.Contains(logger.lastMsg, "Request started"))
}

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware"
)

func TestAPIKeyMiddleware_EmptyValidKeys(t *testing.T) {
	logger := &fakes.MockLogger{}
	_, err := middleware.APIKeyMiddleware(logger, nil)
	require.Error(t, err)

	_, err = middleware.APIKeyMiddleware(logger, []string{})
	require.Error(t, err)
}

func TestAPIKeyMiddleware_MissingKey(t *testing.T) {
	e := echo.New()
	logger := &fakes.MockLogger{}

	apiKeyMW, err := middleware.APIKeyMiddleware(logger, []string{"secret"})
	require.NoError(t, err)

	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(apiKeyMW)

	e.GET("/protected", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAPIKeyMiddleware_InvalidKey(t *testing.T) {
	e := echo.New()
	logger := &fakes.MockLogger{}

	apiKeyMW, err := middleware.APIKeyMiddleware(logger, []string{"expected-secret"})
	require.NoError(t, err)

	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(apiKeyMW)

	e.GET("/protected", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("X-Api-Key", "wrong")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAPIKeyMiddleware_ValidKey(t *testing.T) {
	e := echo.New()
	logger := &fakes.MockLogger{}
	valid := "my-api-key"

	apiKeyMW, err := middleware.APIKeyMiddleware(logger, []string{valid, "other"})
	require.NoError(t, err)

	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(apiKeyMW)

	e.GET("/protected", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("X-Api-Key", valid)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAPIKeyMiddleware_SkipsOptions(t *testing.T) {
	e := echo.New()
	logger := &fakes.MockLogger{}

	apiKeyMW, err := middleware.APIKeyMiddleware(logger, []string{"secret"})
	require.NoError(t, err)

	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(apiKeyMW)

	e.OPTIONS("/protected", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodOptions, "/protected", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

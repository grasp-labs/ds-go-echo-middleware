package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/adapters"
)

func TestAuthX(t *testing.T) {
	e := echo.New()
	cfg := fakes.NewConfig("dp", "core", "fake", "v1.0.0-alpha.1", uuid.New(), 1024)
	logger := &fakes.MockLogger{}
	mockProducer := fakes.MockProducer{}
	mockProducerAdapter := &adapters.ProducerAdapter{
		Producer: &mockProducer,
	}
	topic := "ds.test.test.v1"

	_, publicKeyPEM, err := fakes.GenerateRSAPairPEM()
	if err != nil {
		t.Fatal(err)
	}

	e.Use(middleware.RequestIDMiddleware(logger))
	authX, err := middleware.AuthenticationMiddleware(
		cfg,
		logger,
		publicKeyPEM,
		mockProducerAdapter,
		topic,
	)
	require.NoError(t, err)

	e.Use(authX)

	mwc := []echo.MiddlewareFunc{
		middleware.RequestIDMiddleware(logger),
		authX,
	}
	e.GET("/protected/", func(c echo.Context) error {
		return c.JSON(http.StatusOK, nil)
	}, mwc...)

	req := httptest.NewRequest(http.MethodGet, "/protected/", nil)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

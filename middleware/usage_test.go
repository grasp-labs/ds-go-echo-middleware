package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
)

func TestUsageMiddleware_BasicFlow(t *testing.T) {
	e := echo.New()

	// mocks required for middleware
	cfg := &mockConfig{
		name:          "UsageTestService",
		productID:     uuid.MustParse("7d008b79-f262-4efe-9fbb-e43532625d35"),
		memoryLimitMB: 512,
	}
	logger := &mockLogger{}
	producer := &mockProducer{}

	// Use Middleware under test
	e.Use(middleware.UsageMiddleware(cfg, logger, producer))
	e.Use(middleware.RequestIDMiddleware(logger))

	// Define handler that sets userContext
	// usually set by authentication middleware
	e.GET("api/usage/v1/", func(c echo.Context) error {
		c.Set("userContext", &models.Context{
			Sub: "user@email.com",
		})
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Let client define Request ID
	requestID := uuid.New()

	// Prepare request
	req := httptest.NewRequest(http.MethodGet, "/api/usage/v1/", nil)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Request-ID", requestID.String())

	rec := httptest.NewRecorder()

	// Execute
	e.ServeHTTP(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, producer.called, "Producer should be called")

	// Check usage fields
	entry, ok := producer.value.(models.UsageEntry)
	assert.True(t, ok, "Producer value should be UsageEntry")
	assert.Equal(t, entry.ID, requestID)
	assert.Equal(t, cfg.ProductID(), entry.ProductID)
	assert.Equal(t, cfg.MemoryLimitMB(), entry.MemoryMB)
	assert.NotEmpty(t, entry.ProductID, "ProductID should be set")
}

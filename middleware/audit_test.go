package middleware_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
)

func TestAuditMiddleware_BasicFlow(t *testing.T) {
	e := echo.New()

	// Mocks required for middleware
	cfg := &mockConfig{name: "AuditTestService"}
	logger := &mockLogger{}
	producer := &mockProducer{}

	// Use Middleware under test
	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(middleware.AuditMiddleware(cfg, logger, producer))

	// Define handler that sets userContext
	// usually set by authentication middleware
	e.POST("/api/audit/v1/", func(c echo.Context) error {
		c.Set("userContext", &models.Context{
			Sub: "user@email.com",
			Jti: uuid.MustParse("2cf1d234-9890-40e9-bd68-b323cd9da0e3"),
			Rsc: "MockName:11111111-1111-1111-1111-111111111111",
		})
		return c.JSON(http.StatusCreated, map[string]string{"status": "ok"})
	})

	// Let client define Request ID
	requestID := uuid.New()

	// Prepare request
	body := map[string]string{"foo": "bar"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/audit/v1/", bytes.NewReader(bodyBytes))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Request-ID", requestID.String())

	rec := httptest.NewRecorder()

	// Execute
	e.ServeHTTP(rec, req)

	// Assertions
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.True(t, producer.called, "Producer should have been called")
	assert.NotEmpty(t, producer.key, "Audit key (request ID) should be set")

	// Check audit fields
	entry, ok := producer.value.(models.AuditEntry)
	assert.True(t, ok, "Producer value should be an AuditEntry")
	assert.Equal(t, requestID, entry.ID)
	assert.Equal(t, "POST", entry.HTTPMethod)
	assert.Equal(t, "api", entry.Resource)
	assert.Equal(t, "/api/audit/v1/", entry.Endpoint)
	assert.Equal(t, cfg.Name(), entry.Service)
	assert.Equal(t, entry.Subject, "user@email.com")
	raw, ok := entry.Payload.(json.RawMessage)
	assert.True(t, ok, "Payload should be json.RawMessage")

	assert.JSONEq(t, string(raw), string(bodyBytes))
}

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

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
)

func TestAuditMiddleware_BasicFlow(t *testing.T) {
	e := echo.New()

	// Mocks required for middleware
	cfg := &mockConfig{name: "AuditTestService"}
	logger := &mockLogger{}
	mock := &mockProducer{}
	producer := &adapters.ProducerAdapter{
		Producer: mock,
	}

	// Use Middleware under test
	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(middleware.AuditMiddleware(cfg, logger, producer))

	// Define handler that sets userContext
	// usually set by authentication middleware
	e.POST("/api/audit/v1/", func(c echo.Context) error {
		resourceUUID := uuid.New()
		userCtx := NewTestUserContext("user@email.com", resourceUUID.String()+":MockName")
		c.Set("userContext", userCtx)
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
	assert.True(t, mock.called, "Producer should have been called")
	assert.NotEmpty(t, mock.key, "Audit key (request ID) should be set")

	// Check that the value is EventJson (not AuditEntry directly)
	eventJson, ok := mock.value.(sdkmodels.EventJson)
	assert.True(t, ok, "Producer value should be an EventJson, got %T", mock.value)

	// Verify EventJson structure
	assert.Equal(t, requestID, eventJson.Id)
	// EventType and EventSource appear to be empty strings in your middleware
	// You might want to update your middleware to set these properly
	assert.NotNil(t, eventJson.Payload)

	// Extract payload and verify audit-specific fields
	assert.NotNil(t, eventJson.Payload, "Payload should not be nil")

	payloadMap := *eventJson.Payload
	assert.Equal(t, "POST", payloadMap["http_method"])
	assert.Equal(t, "api", payloadMap["resource"])
	assert.Equal(t, "/api/audit/v1/", payloadMap["endpoint"])
	assert.Equal(t, "/api/audit/v1/", payloadMap["full_url"])
	assert.Equal(t, "user@email.com", payloadMap["subject"])
	assert.Equal(t, cfg.Name(), eventJson.EventSource)

	// Check request body in payload - it's stored as "payload" field with json.RawMessage
	if payloadInterface, exists := payloadMap["payload"]; exists {
		payloadRaw, ok := payloadInterface.(json.RawMessage)
		assert.True(t, ok, "Payload should be json.RawMessage")
		assert.JSONEq(t, string(bodyBytes), string(payloadRaw))
	}
}

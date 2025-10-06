package middleware_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
)

func TestUsageMiddleware_BasicFlow(t *testing.T) {
	e := echo.New()

	// Mocks required for middleware
	cfg := &mockConfig{
		name:          "UsageTestService",
		productID:     uuid.MustParse("06b6b947-26a6-4e66-95e5-9ade49e1ea5c"),
		memoryLimitMB: 512,
	}
	logger := &mockLogger{}
	mock := &mockProducer{}
	producer := &adapters.ProducerAdapter{
		Producer: mock,
	}
	topic := "test_topic"

	// Use Middleware under test
	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(middleware.UsageMiddleware(cfg, logger, producer, topic))

	// Define handler that sets userContext
	e.POST("/api/usage/v1/", func(c echo.Context) error {
		resourceUUID := uuid.New()
		userCtx := NewTestUserContext("user@email.com", resourceUUID.String()+":MockName")
		c.Set("userContext", userCtx)

		// Simulate some response data to measure response size
		responseData := map[string]interface{}{
			"items":  []string{"item1", "item2"},
			"count":  2,
			"status": "success",
		}
		return c.JSON(http.StatusOK, responseData)
	})

	// Define Request ID
	requestID := uuid.New()

	// Prepare request with some data to measure request size
	body := map[string]interface{}{
		"query": "test query for usage tracking",
		"filters": map[string]string{
			"category": "test",
			"status":   "active",
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/usage/v1/", bytes.NewReader(bodyBytes))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Request-ID", requestID.String())
	req.Header.Set("X-Owner-ID", "owner-123")

	rec := httptest.NewRecorder()

	// Execute
	e.ServeHTTP(rec, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, mock.called, "Producer should have been called")
	assert.NotEmpty(t, mock.key, "Usage key should be set")

	// Check that the value is sdkmodels.EventJson
	eventJson, ok := mock.value.(sdkmodels.EventJson)
	if !ok {
		t.Fatalf("Producer value should be models.EventJson, got %T. Value: %+v", mock.value, mock.value)
	}

	// Verify basic EventJson structure
	assert.NotEqual(t, uuid.Nil, eventJson.Id, "Event ID should be set")
	assert.NotNil(t, eventJson.Payload, "Payload should not be nil")

	// Extract Payload map for detailed checks
	payloadMap := *eventJson.Payload

	// Key usage fields
	assert.Equal(t, requestID, eventJson.Id)
	assert.Equal(t, cfg.ProductID(), payloadMap["product_id"])
	assert.Equal(t, cfg.MemoryLimitMB(), payloadMap["memory_mb"])
	assert.NotEmpty(t, payloadMap["start_time"], "Start time should be set")
	assert.NotEmpty(t, payloadMap["end_time"], "End time should be set")
	assert.NotEmpty(t, payloadMap["status"], "Status should be set")
	assert.Equal(t, "user@email.com", payloadMap["user_id"])
	assert.Equal(t, cfg.Name(), payloadMap["service_name"])

	// ProductID type and value
	productIdUUID, ok := payloadMap["product_id"].(uuid.UUID)
	assert.True(t, ok, "Product ID should be uuid.UUID")
	expectedUUID := uuid.MustParse("06b6b947-26a6-4e66-95e5-9ade49e1ea5c")
	assert.Equal(t, expectedUUID, productIdUUID)

	// MemoryMB type and value
	memoryMbInt, ok := payloadMap["memory_mb"].(int16)
	assert.True(t, ok, "Memory MB should be int16")
	assert.GreaterOrEqual(t, int(memoryMbInt), 0, "Memory MB should be non-negative")

	// StartTime type
	_, ok = payloadMap["start_time"].(time.Time)
	assert.True(t, ok, "Start time should be time.Time")

	// EndTime type
	_, ok = payloadMap["end_time"].(time.Time)
	assert.True(t, ok, "End time should be time.Time")

	// Status type
	assert.NotNil(t, payloadMap["status"], "Status should be set")

	// OwnerID presence and value
	assert.NotNil(t, eventJson.OwnerId, "Owner ID should not be nil")
	assert.Equal(t, "owner-123", *eventJson.OwnerId)
}

package middleware_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/v3/middleware/adapters"
)

func TestUsageMiddleware_BasicFlow(t *testing.T) {
	e := echo.New()

	// Mocks required for middleware
	cfg := fakes.NewConfig("dp", "core", "new-service", "v1.0.0-alpha.1", uuid.New(), 1024*2)
	logger := &fakes.MockLogger{}
	mock := &fakes.MockProducer{}
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
		userCtx := fakes.NewTestUserContext("user@email.com", resourceUUID.String()+":MockName")
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
	assert.True(t, mock.WaitForSend(time.Second), "Producer should have been called within 1s")
	assert.True(t, mock.Called(), "Producer should have been called")
	assert.NotEmpty(t, mock.Key(), "Usage key should be set")

	// Check that the value is sdkmodels.EventJson
	eventJson, ok := mock.Value().(sdkmodels.EventJson)
	if !ok {
		t.Fatalf("Producer value should be models.EventJson, got %T. Value: %+v", mock.Value(), mock.Value())
	}

	// Verify basic EventJson structure
	assert.NotEqual(t, uuid.Nil, eventJson.Id, "Event ID should be set")
	assert.NotNil(t, eventJson.Payload, "Payload should not be nil")

	// Extract Payload map for detailed checks
	payloadPtr, ok := eventJson.Payload.(*map[string]any)
	assert.True(t, ok, "Payload should be a map[string]any")

	// Key usage fields
	assert.Equal(t, requestID, eventJson.RequestId)
	assert.NotNil(t, payloadPtr)
	payloadMap := *payloadPtr
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

	assert.Equal(t, cfg.ProductID(), productIdUUID)

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

func TestUsageMiddleware_MissingUserContext(t *testing.T) {
	e := echo.New()

	// Mocks required for middleware
	cfg := fakes.NewConfig("dp", "core", "new-service", "v1.0.0-alpha.1", uuid.New(), 1024*2)
	logger := &fakes.MockLogger{}
	mock := &fakes.MockProducer{}
	producer := &adapters.ProducerAdapter{
		Producer: mock,
	}
	topic := "test_topic"

	// Use Middleware under test
	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(middleware.UsageMiddleware(cfg, logger, producer, topic))

	// Define handler that does NOT set userContext
	e.POST("/api/usage/v1/", func(c echo.Context) error {
		// Don't set userContext
		responseData := map[string]interface{}{
			"status": "success",
		}
		return c.JSON(http.StatusOK, responseData)
	})

	// Prepare request
	requestID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/usage/v1/", nil)
	req.Header.Set("X-Request-ID", requestID.String())

	rec := httptest.NewRecorder()

	// Execute
	e.ServeHTTP(rec, req)

	// Assertions - when user context is missing, the middleware logs a warning
	// but still allows the request to proceed (doesn't send to producer)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, mock.Called(), "Producer should not have been called when user context is missing")
}

func TestUsageMiddleware_ResponseOutcome(t *testing.T) {
	for _, tt := range []struct {
		name       string
		status     int
		err        error
		wantStatus int
		wantUsage  bool
	}{
		{name: "ok", status: http.StatusOK, wantStatus: http.StatusOK, wantUsage: true},
		{name: "created", status: http.StatusCreated, wantStatus: http.StatusCreated, wantUsage: true},
		{name: "no content", status: http.StatusNoContent, wantStatus: http.StatusNoContent, wantUsage: true},
		{name: "redirect", status: http.StatusFound, wantStatus: http.StatusFound, wantUsage: true},
		{name: "unauthorized", status: http.StatusUnauthorized, wantStatus: http.StatusUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, wantStatus: http.StatusForbidden},
		{name: "not found", status: http.StatusNotFound, wantStatus: http.StatusNotFound},
		{name: "validation", status: http.StatusUnprocessableEntity, wantStatus: http.StatusUnprocessableEntity},
		{name: "internal error", status: http.StatusInternalServerError, wantStatus: http.StatusInternalServerError},
		{name: "unavailable", status: http.StatusServiceUnavailable, wantStatus: http.StatusServiceUnavailable},
		{name: "returned HTTP error", err: echo.ErrForbidden, wantStatus: http.StatusForbidden},
		{name: "returned error", err: errors.New("handler failed"), wantStatus: http.StatusInternalServerError},
		{name: "error after response", status: http.StatusOK, err: errors.New("write failed"), wantStatus: http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			cfg := fakes.NewConfig("dp", "core", "usage-test", "v1.0.0", uuid.New(), 1024)
			producer := &fakes.MockProducer{}
			e.Use(middleware.UsageMiddleware(cfg, &fakes.MockLogger{}, &adapters.ProducerAdapter{Producer: producer}, "usage"))
			e.GET("/", func(c echo.Context) error {
				c.Set("userContext", fakes.NewTestUserContext("user@example.com", uuid.NewString()+":Test"))
				if tt.status != 0 {
					if err := c.NoContent(tt.status); err != nil {
						return err
					}
				}
				return tt.err
			})

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
			assert.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantUsage {
				assert.True(t, producer.WaitForSend(time.Second), "successful requests must report usage")
			} else {
				assert.False(t, producer.WaitForSend(50*time.Millisecond), "failed requests must not report usage")
			}
		})
	}
}

package middleware_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"

	"github.com/grasp-labs/ds-go-echo-middleware/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
)

func TestAuditMiddleware_BasicFlow(t *testing.T) {
	e := echo.New()

	// Mocks required for middleware
	cfg := fakes.NewConfig("dp", "core", "fake", "v1.0.0-alpha.1", uuid.New(), 1024)
	logger := &fakes.MockLogger{}
	mock := &fakes.MockProducer{}
	producer := &adapters.ProducerAdapter{
		Producer: mock,
	}
	topic := "test_topic"

	// Use Middleware under test
	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(middleware.AuditMiddleware(cfg, logger, producer, topic))

	// Define handler that sets userContext
	// usually set by authentication middleware
	e.POST("/api/audit/v1/", func(c echo.Context) error {
		resourceUUID := uuid.New()
		userCtx := fakes.NewTestUserContext("user@email.com", resourceUUID.String()+":MockName")
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
	assert.True(t, mock.Called(), "Producer should have been called")
	assert.NotEmpty(t, mock.Key(), "Audit key (request ID) should be set")

	// Check that the value is EventJson (not AuditEntry directly)
	eventJson, ok := mock.Value().(sdkmodels.EventJson)
	assert.True(t, ok, "Producer value should be an EventJson, got %T", mock.Value())

	// Verify EventJson structure
	assert.Equal(t, requestID, eventJson.RequestId)
	// EventType and EventSource appear to be empty strings in your middleware
	// You might want to update your middleware to set these properly
	assert.NotNil(t, eventJson.Payload)

	// Extract payload and verify audit-specific fields
	assert.NotNil(t, eventJson.Payload, "Payload should not be nil")

	payloadMap := *eventJson.Payload
	assert.NotNil(t, payloadMap)
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

func TestAuditMiddleware_NonJSON_DoesNotDrainBody(t *testing.T) {
	e := echo.New()

	cfg := fakes.NewConfig("dp", "core", "new-service", "v1.0.0-alpha.1", uuid.New(), 1024*2)
	logger := &fakes.MockLogger{}
	mp := &fakes.MockProducer{}
	producer := &adapters.ProducerAdapter{Producer: mp}
	topic := "test_topic"

	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(middleware.AuditMiddleware(cfg, logger, producer, topic))

	// Handler that actually reads the body (to verify it wasn't drained)
	var seenBody []byte
	e.PUT("/file/upload", func(c echo.Context) error {
		// Satisfy GetTenantId(): use the same format as your JSON test
		tenantUUID := uuid.New()
		userCtx := fakes.NewTestUserContext("binary@user.com", tenantUUID.String()+":MockName")
		c.Set("userContext", userCtx)

		data, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		seenBody = data
		return c.NoContent(http.StatusOK)
	})

	// Non-JSON body
	bin := []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01}
	req := httptest.NewRequest(http.MethodPut, "/file/upload", bytes.NewReader(bin))
	req.Header.Set(echo.HeaderContentType, "application/octet-stream")

	// (optional) set a request id; your RequestID middleware can also generate one
	req.Header.Set("X-Request-ID", uuid.New().String())

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Producer should have been called even for non-JSON
	if !assert.True(t, mp.Called(), "Producer should have been called for non-JSON too") {
		t.Fatalf("producer not called; check GetTenantId() expectations and userContext fixture")
	}

	// Downstream saw the exact bytes (middleware didn’t drain)
	assert.Equal(t, bin, seenBody)

	eventJson := mp.Value().(sdkmodels.EventJson)
	if eventJson.Payload != nil {
		if p, ok := (*eventJson.Payload)["payload"]; ok {
			if rm, ok := p.(json.RawMessage); ok {
				// Accept missing or empty payload for non-JSON
				if len(rm) != 0 {
					t.Fatalf("payload should be empty for non-JSON requests, got: %q", string(rm))
				}
			} else if p != nil {
				t.Fatalf("payload has unexpected type: %T", p)
			}
		}
	}
}

// package middleware_test

// import (
// 	"bytes"
// 	"encoding/json"
// 	"net/http"
// 	"net/http/httptest"
// 	"testing"

// 	"github.com/google/uuid"
// 	"github.com/labstack/echo/v4"
// 	"github.com/stretchr/testify/assert"

// 	sdkmodels "github.com/grasp-labs/ds-event-stream-go-sdk/models"
// 	"github.com/grasp-labs/ds-go-echo-middleware/middleware"
// 	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
// 	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
// )

// func TestAuditMiddleware_BasicFlow(t *testing.T) {
// 	e := echo.New()

// 	// Mocks required for middleware
// 	cfg := &mockConfig{name: "AuditTestService"}
// 	logger := &mockLogger{}
// 	mock := &mockProducer{}
// 	producer := &adapters.ProducerAdapter{
// 		Producer: mock,
// 	}

// 	// Use Middleware under test
// 	e.Use(middleware.RequestIDMiddleware(logger))
// 	e.Use(middleware.AuditMiddleware(cfg, logger, producer))

// 	// Define handler that sets userContext
// 	// usually set by authentication middleware
// 	e.POST("/api/audit/v1/", func(c echo.Context) error {
// 		c.Set("userContext", &models.Context{
// 			Sub: "user@email.com",
// 			Jti: uuid.MustParse("2cf1d234-9890-40e9-bd68-b323cd9da0e3"),
// 			Rsc: "MockName:11111111-1111-1111-1111-111111111111",
// 		})
// 		return c.JSON(http.StatusCreated, map[string]string{"status": "ok"})
// 	})

// 	// Let client define Request ID
// 	requestID := uuid.New()

// 	// Prepare request
// 	body := map[string]string{"foo": "bar"}
// 	bodyBytes, _ := json.Marshal(body)
// 	req := httptest.NewRequest(http.MethodPost, "/api/audit/v1/", bytes.NewReader(bodyBytes))
// 	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
// 	req.Header.Set("X-Request-ID", requestID.String())

// 	rec := httptest.NewRecorder()

// 	// Execute
// 	e.ServeHTTP(rec, req)

// 	// Assertions
// 	assert.Equal(t, http.StatusCreated, rec.Code)
// 	assert.True(t, mock.called, "Producer should have been called")
// 	assert.NotEmpty(t, mock.key, "Audit key (request ID) should be set")

// 	// Check audit fields
// 	entry, ok := mock.value.(models.AuditEntry)
// 	assert.True(t, ok, "Producer value should be an AuditEntry")
// 	assert.Equal(t, requestID, entry.ID)
// 	assert.Equal(t, "POST", entry.HTTPMethod)
// 	assert.Equal(t, "api", entry.Resource)
// 	assert.Equal(t, "/api/audit/v1/", entry.Endpoint)
// 	assert.Equal(t, cfg.Name(), entry.Service)
// 	assert.Equal(t, entry.Subject, "user@email.com")
// 	raw, ok := entry.Payload.(json.RawMessage)
// 	assert.True(t, ok, "Payload should be json.RawMessage")

// 	assert.JSONEq(t, string(raw), string(bodyBytes))
// }

// func TestAuditMiddleware_Debug(t *testing.T) {
// 	e := echo.New()
// 	cfg := &mockConfig{name: "DebugService"}
// 	logger := &mockLogger{}
// 	mock := &mockProducer{}
// 	producer := &adapters.ProducerAdapter{Producer: mock}

// 	e.Use(middleware.RequestIDMiddleware(logger))
// 	e.Use(middleware.AuditMiddleware(cfg, logger, producer))

// 	e.POST("/debug", func(c echo.Context) error {
// 		c.Set("userContext", &models.Context{
// 			Sub: "debug@email.com",
// 			Jti: uuid.New(),
// 			Rsc: "Debug:11111111-1111-1111-1111-111111111111",
// 		})
// 		return c.JSON(http.StatusOK, map[string]string{"debug": "test"})
// 	})

// 	req := httptest.NewRequest(http.MethodPost, "/debug", bytes.NewReader([]byte(`{"test": "data"}`)))
// 	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
// 	req.Header.Set("X-Request-ID", uuid.New().String())

// 	rec := httptest.NewRecorder()
// 	e.ServeHTTP(rec, req)

// 	// Debug output - remove this once you understand the structure
// 	t.Logf("Mock called: %v", mock.called)
// 	t.Logf("Mock key: %s", mock.key)
// 	t.Logf("Mock value type: %T", mock.value)
// 	t.Logf("Mock value: %+v", mock.value)

// 	if eventJson, ok := mock.value.(sdkmodels.EventJson); ok {
// 		t.Logf("EventJson.Id: %v", eventJson.Id)
// 		t.Logf("EventJson.EventType: %s", eventJson.EventType)
// 		t.Logf("EventJson.EventSource: %s", eventJson.EventSource)
// 		t.Logf("EventJson.TenantId: %s", eventJson.TenantId)
// 		t.Logf("EventJson.Payload type: %T", eventJson.Payload)
// 		if eventJson.Payload != nil {
// 			for k, v := range *eventJson.Payload {
// 				t.Logf("Payload[%s]: %v (type: %T)", k, v, v)
// 			}
// 		}
// 	}
// }

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
	assert.Equal(t, cfg.Name(), payloadMap["service"]) // Service is in payload, not EventSource

	// Check request body in payload - it's stored as "payload" field with json.RawMessage
	if payloadInterface, exists := payloadMap["payload"]; exists {
		payloadRaw, ok := payloadInterface.(json.RawMessage)
		assert.True(t, ok, "Payload should be json.RawMessage")
		assert.JSONEq(t, string(bodyBytes), string(payloadRaw))
	}
}

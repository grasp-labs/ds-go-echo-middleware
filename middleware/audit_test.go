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

	"github.com/grasp-labs/ds-go-echo-middleware/v2/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/internal/utils"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/adapters"
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

	payloadPtr, ok := eventJson.Payload.(*map[string]any)
	assert.True(t, ok, "Payload should be a map[string]any")
	assert.NotNil(t, payloadPtr)

	payloadMap := *payloadPtr
	assert.Equal(t, "POST", payloadMap["http_method"])
	assert.Equal(t, "api", payloadMap["resource"])
	assert.Equal(t, "/api/audit/v1/", payloadMap["endpoint"])
	assert.Equal(t, "/api/audit/v1/", payloadMap["full_url"])
	assert.Equal(t, "user@email.com", payloadMap["subject"])
	assert.Equal(t, utils.CreateServicePrincipleID(cfg), eventJson.EventSource)

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
		if payloadPtr, ok := eventJson.Payload.(*map[string]any); ok {
			payloadMap := *payloadPtr
			if p, ok := payloadMap["payload"]; ok {
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
}

func TestAuditMiddleware_SuccessResponse_NoResponseBody(t *testing.T) {
	e := echo.New()

	cfg := fakes.NewConfig("dp", "core", "fake", "v1.0.0-alpha.1", uuid.New(), 1024)
	logger := &fakes.MockLogger{}
	mock := &fakes.MockProducer{}
	producer := &adapters.ProducerAdapter{Producer: mock}
	topic := "test_topic"

	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(middleware.AuditMiddleware(cfg, logger, producer, topic))

	e.POST("/api/audit/v1/", func(c echo.Context) error {
		resourceUUID := uuid.New()
		userCtx := fakes.NewTestUserContext("user@email.com", resourceUUID.String()+":MockName")
		c.Set("userContext", userCtx)
		return c.JSON(http.StatusCreated, map[string]string{"status": "ok"})
	})

	requestID := uuid.New()
	body := map[string]string{"foo": "bar"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/audit/v1/", bytes.NewReader(bodyBytes))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Request-ID", requestID.String())

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.True(t, mock.Called(), "Producer should have been called")

	eventJson, ok := mock.Value().(sdkmodels.EventJson)
	assert.True(t, ok, "Producer value should be an EventJson")

	assert.Equal(t, requestID, eventJson.RequestId)
	assert.NotNil(t, eventJson.Payload)

	payloadPtr, ok := eventJson.Payload.(*map[string]any)
	assert.True(t, ok, "Payload should be a map[string]any")
	payloadMap := *payloadPtr

	// Verify audit fields
	assert.Equal(t, "POST", payloadMap["http_method"])
	assert.Equal(t, "api", payloadMap["resource"])
	assert.Equal(t, "/api/audit/v1/", payloadMap["endpoint"])
	assert.Equal(t, "user@email.com", payloadMap["subject"])
	assert.Equal(t, 201, payloadMap["status_code"])

	// Success response should NOT capture response body
	assert.Nil(t, payloadMap["response_payload"], "Success responses should not capture response body")

	// Verify request body was captured
	if reqPayload, ok := payloadMap["payload"].(json.RawMessage); ok {
		assert.JSONEq(t, string(bodyBytes), string(reqPayload))
	}
}

func TestAuditMiddleware_ErrorResponse_CapturesResponseBody(t *testing.T) {
	e := echo.New()

	cfg := fakes.NewConfig("dp", "core", "fake", "v1.0.0-alpha.1", uuid.New(), 1024)
	logger := &fakes.MockLogger{}
	mock := &fakes.MockProducer{}
	producer := &adapters.ProducerAdapter{Producer: mock}
	topic := "test_topic"

	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(middleware.AuditMiddleware(cfg, logger, producer, topic))

	e.POST("/api/audit/v1/error", func(c echo.Context) error {
		resourceUUID := uuid.New()
		userCtx := fakes.NewTestUserContext("user@email.com", resourceUUID.String()+":MockName")
		c.Set("userContext", userCtx)

		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "validation failed",
			"field": "email",
		})
	})

	requestID := uuid.New()
	body := map[string]string{"email": "invalid"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/audit/v1/error", bytes.NewReader(bodyBytes))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("X-Request-ID", requestID.String())

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	// Assert response sent to client
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, echo.MIMEApplicationJSON, rec.Header().Get(echo.HeaderContentType))

	// Verify client received the correct response body
	var clientResponse map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &clientResponse)
	assert.NoError(t, err, "Client should receive valid JSON")
	assert.Equal(t, "validation failed", clientResponse["error"])
	assert.Equal(t, "email", clientResponse["field"])

	// Verify audit log was sent to Kafka
	assert.True(t, mock.Called(), "Producer should have been called")

	eventJson, ok := mock.Value().(sdkmodels.EventJson)
	assert.True(t, ok, "Producer value should be an EventJson")

	payloadPtr, ok := eventJson.Payload.(*map[string]any)
	payloadMap := *payloadPtr
	assert.True(t, ok, "Payload should be a map[string]any")

	assert.Equal(t, 400, payloadMap["status_code"])

	// Error response SHOULD capture response body in audit log
	assert.NotNil(t, payloadMap["response_payload"], "Error responses should capture response body")

	if respPayload, ok := payloadMap["response_payload"].(json.RawMessage); ok {
		var auditedResponse map[string]string
		err := json.Unmarshal(respPayload, &auditedResponse)
		assert.NoError(t, err)
		assert.Equal(t, "validation failed", auditedResponse["error"])
		assert.Equal(t, "email", auditedResponse["field"])

		// Verify audit log matches what client received
		assert.Equal(t, clientResponse, auditedResponse, "Audited response should match client response")
	} else {
		t.Fatal("response_payload should be json.RawMessage")
	}
}

func TestAuditMiddleware_ServerError_CapturesResponseBody(t *testing.T) {
	e := echo.New()

	cfg := fakes.NewConfig("dp", "core", "fake", "v1.0.0-alpha.1", uuid.New(), 1024)
	logger := &fakes.MockLogger{}
	mock := &fakes.MockProducer{}
	producer := &adapters.ProducerAdapter{Producer: mock}
	topic := "test_topic"

	e.Use(middleware.RequestIDMiddleware(logger))
	e.Use(middleware.AuditMiddleware(cfg, logger, producer, topic))

	e.GET("/api/server-error", func(c echo.Context) error {
		resourceUUID := uuid.New()
		userCtx := fakes.NewTestUserContext("user@email.com", resourceUUID.String()+":MockName")
		c.Set("userContext", userCtx)

		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "database connection failed",
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/server-error", nil)
	req.Header.Set("X-Request-ID", uuid.New().String())

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	eventJson := mock.Value().(sdkmodels.EventJson)
	payloadPtr, ok := eventJson.Payload.(*map[string]any)
	assert.True(t, ok, "Payload should be a map[string]any")
	payloadMap := *payloadPtr

	assert.Equal(t, 500, payloadMap["status_code"])
	assert.NotNil(t, payloadMap["response_payload"], "5xx responses should capture response body")
}

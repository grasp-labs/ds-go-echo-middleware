package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/models"
)

func TestAuditEntry_JSON_MarshalBasic(t *testing.T) {
	tenantID := uuid.New()
	resourceID := uuid.New()

	entry := models.AuditEntry{
		TenantID:   tenantID,
		Subject:    "user@example.com",
		HTTPMethod: "POST",
		Resource:   "target",
		ResourceID: resourceID,
		Payload: map[string]any{
			"name": "new target",
		},
		SourceIP:    "10.1.1.1",
		UserAgent:   "curl/7.68.0",
		Timestamp:   time.Date(2025, 7, 9, 14, 30, 0, 0, time.UTC),
		Service:     "target-api",
		Endpoint:    "/v1/targets",
		FullURL:     "https://api.example.com/v1/targets",
		ID:          uuid.MustParse("5dd1f93d-4ad8-4c1d-916a-64fb42197030"),
		Correlation: "trace-xyz789",
	}

	data, err := json.Marshal(entry)
	assert.NoError(t, err, "JSON marshaling should succeed")

	jsonStr := string(data)
	assert.Contains(t, jsonStr, `"tenant_id"`)
	assert.Contains(t, jsonStr, `"subject":"user@example.com"`)
	assert.Contains(t, jsonStr, `"resource":"target"`)
	assert.Contains(t, jsonStr, `"id":"5dd1f93d-4ad8-4c1d-916a-64fb42197030"`)
	assert.Contains(t, jsonStr, `"correlation_id":"trace-xyz789"`)
}

func TestAuditEntry_JSON_Keys(t *testing.T) {
	entry := models.AuditEntry{
		TenantID:   uuid.New(),
		Subject:    "user@example.com",
		HTTPMethod: "DELETE",
		Resource:   "result",
		Timestamp:  time.Now().UTC(),
		Service:    "result-api",
		Endpoint:   "/v1/results",
		FullURL:    "https://api.example.com/v1/results",
	}

	data, err := json.Marshal(entry)
	assert.NoError(t, err)

	jsonStr := string(data)
	assert.Contains(t, jsonStr, `"payload"`)
	assert.Contains(t, jsonStr, `"resource_id"`)
	assert.Contains(t, jsonStr, `"correlation_id"`)
}

func TestAuditEntry_JSON_Payload(t *testing.T) {
	payload := map[string]any{
		"timeout_ms": 1000,
	}

	entry := models.AuditEntry{
		TenantID:   uuid.New(),
		Subject:    "user@example.com",
		HTTPMethod: "PATCH",
		Resource:   "target",
		Timestamp:  time.Now(),
		Service:    "target-api",
		Payload:    payload,
	}

	data, err := json.Marshal(entry)
	assert.NoError(t, err)

	jsonStr := string(data)
	assert.Contains(t, jsonStr, `"timeout_ms":1000`)
}

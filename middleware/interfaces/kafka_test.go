package interfaces_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/grasp-labs/ds-event-stream-go-sdk/models"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/adapters"
)

// mockProducer implements interfaces.Producer
type mockProducer struct {
	called bool
	key    string
	value  any
}

func (m *mockProducer) Send(ctx context.Context, key string, value any) error {
	m.called = true
	m.key = key
	m.value = value
	return nil
}

func (m *mockProducer) Close() error {
	return nil
}

func TestProducerAdapter_Send(t *testing.T) {
	mock := &mockProducer{}
	adapter := &adapters.ProducerAdapter{
		Producer: mock,
	}

	event := models.EventJson{
		Id:          uuid.New(),
		SessionId:   uuid.New(),
		RequestId:   uuid.New(),
		TenantId:    uuid.New(),
		EventType:   "unit.test.v1",
		EventSource: "unit-test",
		Metadata:    map[string]string{"test": "unit"},
		Timestamp:   time.Now(),
		CreatedBy:   "unit-test",
		Md5Hash:     "d41d8cd98f00b204e9800998ecf8427e",
		Payload:     &map[string]interface{}{"test_message": "unit test"},
	}

	err := adapter.Send(context.Background(), "test-key", event)
	assert.NoError(t, err)
	assert.True(t, mock.called, "SendEvent should be called")
	assert.Equal(t, "test-key", mock.key)
	assert.IsType(t, models.EventJson{}, mock.value, "Event should be of type models.EventJson")
}

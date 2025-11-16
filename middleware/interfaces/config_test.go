package interfaces_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/internal/fakes"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/interfaces"
)

func mockCallable(config interfaces.Config) (string, uuid.UUID, int16) {
	return config.Name(), config.ProductID(), config.MemoryLimitMB()
}

func TestConfig_CallableAttributes(t *testing.T) {
	c := fakes.NewConfig("dp", "core", "new-service", "v1.0.0-alpha.1", uuid.New(), 1024*2)

	// Assign to interface
	var cfg interfaces.Config = c

	name, productID, memoryLimitMB := mockCallable(cfg)
	assert.Equal(t, name, c.Name())
	assert.Equal(t, productID, c.ProductID())
	assert.Equal(t, memoryLimitMB, c.MemoryLimitMB())
}

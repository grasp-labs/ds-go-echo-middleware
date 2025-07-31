package interfaces_test

import (
	"testing"

	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/interfaces"
)

type mockConfig struct {
	name          string
	productID     uuid.UUID
	memoryLimitMB int16
	apiCache      *bigcache.BigCache
}

// Implement the interface methods
func (c *mockConfig) ProductID() uuid.UUID {
	return c.productID
}

func (c *mockConfig) Name() string {
	return c.name
}

func (c *mockConfig) MemoryLimitMB() int16 {
	return c.memoryLimitMB
}

func (c *mockConfig) APICache() *bigcache.BigCache {
	return c.apiCache
}

func mockCallable(config interfaces.Config) (string, uuid.UUID, int16) {
	return config.Name(), config.ProductID(), config.MemoryLimitMB()
}

func TestConfig_CallableAttributes(t *testing.T) {
	c := mockConfig{
		name:          "test",
		productID:     uuid.New(),
		memoryLimitMB: 1024,
	}

	// Assign to interface
	var cfg interfaces.Config = &c

	name, productID, memoryLimitMB := mockCallable(cfg)
	assert.Equal(t, name, c.Name())
	assert.Equal(t, productID, c.ProductID())
	assert.Equal(t, memoryLimitMB, c.MemoryLimitMB())
}

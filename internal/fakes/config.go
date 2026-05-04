package fakes

import (
	"context"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"
)

type MockConfig struct {
	domain        string
	serviceGroup  string
	name          string
	version       string
	productID     uuid.UUID
	memoryLimitMB int16
	apiCache      *bigcache.BigCache
}

// Implement the interface methods
func (c *MockConfig) ProductID() uuid.UUID {
	return c.productID
}

func (c *MockConfig) Name() string {
	return c.name
}

func (c *MockConfig) MemoryLimitMB() int16 {
	return c.memoryLimitMB
}

func (c *MockConfig) APICache() *bigcache.BigCache {
	return c.apiCache
}

func (c *MockConfig) Domain() string {
	return c.domain
}

func (c *MockConfig) ServiceGroup() string {
	return c.serviceGroup
}

func (c *MockConfig) Version() string {
	return c.version
}

func NewConfig(d, sg, n, v string, pid uuid.UUID, mb int16) *MockConfig {
	cache, _ := bigcache.New(context.Background(), bigcache.Config{
		Shards:             1,
		LifeWindow:         5 * time.Minute,
		MaxEntriesInWindow: 100,
		MaxEntrySize:       500,
		HardMaxCacheSize:   8,
	})
	return &MockConfig{
		domain:        d,
		serviceGroup:  sg,
		name:          n,
		version:       v,
		productID:     pid,
		memoryLimitMB: mb,
		apiCache:      cache,
	}
}

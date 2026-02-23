package interfaces

import (
	"github.com/google/uuid"

	"github.com/allegro/bigcache/v3"
)

type Config interface {
	MemoryLimitMB() int16
	Domain() string       // New
	ServiceGroup() string // New
	Version() string
	Name() string
	ProductID() uuid.UUID
	APICache() *bigcache.BigCache
}

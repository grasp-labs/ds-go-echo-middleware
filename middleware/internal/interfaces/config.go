package interfaces

import (
	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"
)

type Config interface {
	MemoryLimitMB() int16
	Name() string
	ProductID() uuid.UUID
	APICache() *bigcache.BigCache
}

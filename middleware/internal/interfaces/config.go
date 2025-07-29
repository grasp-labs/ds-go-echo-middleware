package interfaces

import (
	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"
)

type Permission struct {
	Roles []string
	URL   string
}

type Config interface {
	MemoryLimitMB() int16
	Name() string
	ProductID() uuid.UUID
	APICache() *bigcache.BigCache
	Permission() *Permission
}

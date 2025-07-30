# 🧩 Example: Config Struct Implementation

Below you will find a full example of implementing a Config struct that satisfies
a simplified Config interface.

## ✅ Interface Recap

The interface you're satisfying looks like:

```go
type Config interface {
	MemoryLimitMB() int16
	Name() string
	ProductID() uuid.UUID
	APICache() *bigcache.BigCache
	Permission() PermissionConfig
}
```

## 📦 config.go (Example Implementation)

```go
package config

import (
	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/interfaces"
)

type AppConfig struct {
	name          string
	productID     uuid.UUID
	memoryLimitMB int16
	apiCache      *bigcache.BigCache
	permission    interfaces.PermissionConfig
}

// Constructor
func NewAppConfig(name string, productID uuid.UUID, limit int16, cache *bigcache.BigCache, perm *interfaces.PermissionConfig) *AppConfig {
	return &AppConfig{
		name:          name,
		productID:     productID,
		memoryLimitMB: limit,
		apiCache:      cache,
		permission:    perm,
	}
}

// Interface methods
func (c *AppConfig) Name() string {
	return c.name
}

func (c *AppConfig) ProductID() uuid.UUID {
	return c.productID
}

func (c *AppConfig) MemoryLimitMB() int16 {
	return c.memoryLimitMB
}

func (c *AppConfig) APICache() *bigcache.BigCache {
	return c.apiCache
}

func (c *AppConfig) Permission() *interfaces.Permission {
	return c.permission
}
```

## 📦 main.go (Usage Example)

```go
package main

import (
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware"
	"github.com/grasp-labs/ds-go-echo-middleware/middleware/internal/interfaces"
	"yourapp/config"
)

func main() {
	cache, _ := bigcache.NewBigCache(bigcache.DefaultConfig(24 * time.Hour))

	cfg := config.NewAppConfig(
		"your-service",
		uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		1024,
		cache,
		&interfaces.Permission{
			Roles: []string{"admin", "user"},
			URL:   "https://entitlement-api.example.com/v1/user/roles",
		},
	)

	// Now pass cfg into middleware, e.g.:
	// e.Use(middleware.AuthorizationMiddleware(cfg, logger, producer))
}
```
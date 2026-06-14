# 🧩 Example: Config Struct Implementation

This document demonstrates how to implement a `Config` struct that satisfies a simplified `Config` interface, typically required by middleware or shared libraries.

---

## ✅ Interface Recap

The following is the interface you need to satisfy:

```go
type Config interface {
	MemoryLimitMB() int16
	Domain() string       // New
	ServiceGroup() string // New
	Version() string
	Name() string
	ProductID() uuid.UUID
	APICache() *bigcache.BigCache
	Issuer() string // New: this env's identity-server issuer URL
}
```

> **`Issuer()`** returns this environment's identity-server issuer (e.g.
> `https://auth.grasp-daas.com`). The authentication middleware enforces the
> token `iss` against it and, when JWKS is enabled (see the auth opt-in example),
> derives the JWKS URI from it as `{Issuer}/oauth/.well-known/jwks.json`.

Since authorization middleware also require configuration, you may choose to add:

```go
	Permission() PermissionConfig
```

## 📦 config.go (Example Implementation)

```go
package config

import (
	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"
)

// PermissionConfig defines access groups and entitlement service endpoint.
type PermissionConfig struct {
	roles []string
	url   string
}

// Roles returns the allowed access groups.
func (p *PermissionConfig) Roles() []string { return p.roles }

// Url returns the entitlement service URL.
func (p *PermissionConfig) Url() string { return p.url }

// AppConfig implements the Config interface and holds service configuration.
type AppConfig struct {
	domain		  string
	serviceGroup  string
	name          string
	version		  string
	productID     uuid.UUID
	memoryLimitMB int16
	apiCache      *bigcache.BigCache
	permission    PermissionConfig
	issuer        string
}

// NewAppConfig constructs a new AppConfig instance with required fields.
func NewAppConfig(domain, serviceGroup, name, version string, productID uuid.UUID, limit int16, cache *bigcache.BigCache, perm PermissionConfig, issuer string) *AppConfig {
	return &AppConfig{
		domain:        domain,
		serviceGroup:  serviceGroup,
		name:          name,
		version:       version,
		productID:     productID,
		memoryLimitMB: limit,
		apiCache:      cache,
		permission:    perm,
		issuer:        issuer,
	}
}

// Interface method implementations:
func (c *AppConfig) Domain() string {
	return c.domain
}

func (c *AppConfig) ServiceGroup() string {
	return c.serviceGroup
}

func (c *AppConfig) Name() string {
	return c.name
}

func (c *AppConfig) Version() string {
	return c.version
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

func (c *AppConfig) Permission() PermissionConfig {
	return c.permission
}

func (c *AppConfig) Issuer() string {
	return c.issuer
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
	"yourapp/config"
)

func main() {
	cache, _ := bigcache.New(context.Background(), bigcache.DefaultConfig(10 * time.Minute))

	cfg := config.NewAppConfig(
		"dp",
		"core",
		"your-service",
		"v1.0.0-alpha-1",
		uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		1024,
		cache,
		config.PermissionConfig{
			roles: []string{"admin", "user"},
			url:   "https://entitlement-api.example.com/v1/user/roles",
		},
		"https://auth.grasp-daas.com", // issuer (per environment)
	)

	// Pass cfg into middleware setup
	// e.Use(middleware.AuthorizationMiddleware(cfg, logger, producer))
}
```
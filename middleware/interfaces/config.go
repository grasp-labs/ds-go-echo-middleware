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

	// Issuer is this environment's identity-server issuer URL
	// (e.g. "https://auth.grasp-daas.com"). The auth middleware enforces
	// token `iss` against it and derives the JWKS URI from it
	// ({Issuer}/oauth/.well-known/jwks.json).
	Issuer() string
}

package interfaces

import (
	"github.com/allegro/bigcache/v3"
	"github.com/google/uuid"
)

type PermissionConfig struct {
	// roles represents the required group names a user must belong to.
	// Despite the name, these are groups, not roles.
	roles []string

	// url is the full endpoint of the entitlement API used to fetch
	// the user's group memberships for permission checks.
	url string
}

// Roles returns the list of accepted groups (not roles) used for access control.
func (p *PermissionConfig) Roles() []string {
	return p.roles
}

// Url returns the entitlement API endpoint used to resolve group memberships.
func (p *PermissionConfig) Url() string {
	return p.url
}

type Config interface {
	MemoryLimitMB() int16
	Name() string
	ProductID() uuid.UUID
	APICache() *bigcache.BigCache
	Permission() PermissionConfig
}

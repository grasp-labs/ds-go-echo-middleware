package models

import (
	"time"

	"github.com/google/uuid"
)

// Authentication and authorization Event model.
type AuthEvent struct {
	ID          uuid.UUID `json:"id"` // RequestID
	ServiceName string    `json:"service_name"`
	Type        string    `json:"type"`
	Subject     string    `json:"subject"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Error       string    `json:"error"`
	Path        string    `json:"path"`
	UserAgent   string    `json:"user_agent"`
	RemoteAddr  string    `json:"remote_addr"`
	Timestamp   time.Time `json:"timestamp"`
}

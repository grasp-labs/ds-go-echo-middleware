package models

import (
	"time"

	"github.com/google/uuid"
)

type UsageEntry struct {
	ID             uuid.UUID           `json:"id"`
	TenantID       uuid.UUID           `json:"tenant_id"`
	OwnerID        *string             `json:"owner_id"` // pointer to allow null
	ProductID      uuid.UUID           `json:"product_id"`
	MemoryMB       int16               `json:"memory_mb"`
	StartTimestamp time.Time           `json:"start_timestamp"`
	EndTimestamp   time.Time           `json:"end_timestmap"`
	Duration       float64             `json:"duration"`
	Status         ItemStatus          `json:"status"`
	Metadata       []map[string]string `json:"metadata"`
	Tags           map[string]string   `json:"tags"`
	CreatedAt      time.Time           `json:"created_at"`
	CreatedBy      string              `json:"created_by"`
}

func (u *UsageEntry) Validate() []ValidationError {
	var errors []ValidationError
	return errors
}

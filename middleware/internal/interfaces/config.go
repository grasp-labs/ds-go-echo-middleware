package interfaces

import "github.com/google/uuid"

type Config interface {
	MemoryLimitMB() int16
	Name() string
	ProductID() uuid.UUID
}

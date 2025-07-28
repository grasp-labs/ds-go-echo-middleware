package models

// ItemStatus Enum-like constants
type ItemStatus string

const (
	Active    ItemStatus = "active"
	Deleted   ItemStatus = "deleted"
	Suspended ItemStatus = "suspended"
	Rejected  ItemStatus = "rejected"
	Draft     ItemStatus = "draft"
	Closed    ItemStatus = "closed"
)

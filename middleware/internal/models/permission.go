package models

type Permission struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	TenantId string `json:"tenant_id"`
}

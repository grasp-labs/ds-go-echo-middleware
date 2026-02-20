package models_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	internal "github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/internal/models"
)

func TestContext_GetTenantId(t *testing.T) {
	validTenantID := uuid.New().String()
	invalidTenantID := "invalid-uuid"

	tests := []struct {
		name    string
		context internal.Context
		want    string
		wantErr bool
	}{
		{
			name: "valid tenant ID",
			context: internal.Context{
				Rsc: validTenantID + ":tenantName",
			},
			want:    validTenantID,
			wantErr: false,
		},
		{
			name: "invalid tenant ID",
			context: internal.Context{
				Rsc: invalidTenantID + ":tenantName",
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.context.GetTenantId()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got.String())
			}
		})
	}
}

func TestContext_GetTenantName(t *testing.T) {
	validTenantName := "Super Happy Funland"
	invalidTenantName := ""

	tests := []struct {
		name    string
		context internal.Context
		want    string
		wantErr bool
	}{
		{
			name: "valid tenant name",
			context: internal.Context{
				Rsc: uuid.New().String() + ":" + validTenantName,
			},
			want:    validTenantName,
			wantErr: false,
		},
		{
			name: "empty tenant name",
			context: internal.Context{
				Rsc: uuid.New().String() + ":" + invalidTenantName,
			},
			want:    invalidTenantName,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.context.GetTenantName()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContext_Valid(t *testing.T) {

	tests := []struct {
		name    string
		context internal.Context
		want    error
		wantErr bool
	}{
		{
			name: "valid everything",
			context: internal.Context{
				Exp: float64(time.Now().Unix() + 4),
				Nbf: float64(time.Now().Unix()),
				Iat: float64(time.Now().Unix() - 4),
				Iss: "https://auth.grasp-daas.com",
				Sub: "Hit like and subscribe",
				Rsc: uuid.New().String() + ":" + "valid_name",
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "invalid exp",
			context: internal.Context{
				Exp: float64(time.Now().Unix() - 4),
				Nbf: float64(time.Now().Unix()),
				Iat: float64(time.Now().Unix() - 4),
				Iss: "https://auth.grasp-daas.com",
				Sub: "Hit like and subscribe",
				Rsc: uuid.New().String() + ":" + "valid_name",
			},
			want:    errors.New("token has expired"),
			wantErr: true,
		},
		{
			name: "invalid nbf",
			context: internal.Context{
				Exp: float64(time.Now().Unix() + 4),
				Nbf: float64(time.Now().Unix() + 10), // Not before is in the future
				Iat: float64(time.Now().Unix() - 4),
				Iss: "https://auth.grasp-daas.com",
				Sub: "Hit like and subscribe",
				Rsc: uuid.New().String() + ":" + "valid_name",
			},
			want:    errors.New("token not yet valid"),
			wantErr: true,
		},
		{
			name: "invalid iat",
			context: internal.Context{
				Exp: float64(time.Now().Unix() + 4),
				Nbf: float64(time.Now().Unix()),
				Iat: float64(time.Now().Unix() + 10), // Issued at is in the future
				Iss: "https://auth.grasp-daas.com",
				Sub: "Hit like and subscribe",
				Rsc: uuid.New().String() + ":" + "valid_name",
			},
			want:    errors.New("token issued in the future"),
			wantErr: true,
		},
		{
			name: "invalid issuer",
			context: internal.Context{
				Exp: float64(time.Now().Unix() + 4),
				Nbf: float64(time.Now().Unix()),
				Iat: float64(time.Now().Unix() - 4),
				Iss: "https://invalid-issuer.com", // Invalid issuer
				Sub: "Hit like and subscribe",
				Rsc: uuid.New().String() + ":" + "valid_name",
			},
			want:    errors.New("invalid issuer"),
			wantErr: true,
		},
		{
			name: "missing sub",
			context: internal.Context{
				Exp: float64(time.Now().Unix() + 4),
				Nbf: float64(time.Now().Unix()),
				Iat: float64(time.Now().Unix() - 4),
				Iss: "https://auth.grasp-daas.com",
				Sub: "", // Missing subject
				Rsc: uuid.New().String() + ":" + "valid_name",
			},
			want:    errors.New("invalid sub"),
			wantErr: true,
		},
		{
			name: "invalid resource format",
			context: internal.Context{
				Exp: float64(time.Now().Unix() + 4),
				Nbf: float64(time.Now().Unix()),
				Iat: float64(time.Now().Unix() - 4),
				Iss: "https://auth.grasp-daas.com",
				Sub: "Hit like and subscribe",
				Rsc: "invalid_resource_format", // Invalid resource format
			},
			want:    errors.New("invalid resource"),
			wantErr: true,
		},
		{
			name: "invalid resource UUID",
			context: internal.Context{
				Exp: float64(time.Now().Unix() + 4),
				Nbf: float64(time.Now().Unix()),
				Iat: float64(time.Now().Unix() - 4),
				Iss: "https://auth.grasp-daas.com",
				Sub: "Hit like and subscribe",
				Rsc: "invalid-uuid:valid_name", // Invalid UUID in resource
			},
			want:    errors.New("invalid resource"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.context.Valid()
			assert.Equal(t, tt.want, got)
		})
	}

}

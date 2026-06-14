package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

type Context struct {
	Iss string    `json:"iss"` // Issuer
	Sub string    `json:"sub"` // Subject
	Aud []string  `json:"aud"` // Audience
	Exp float64   `json:"exp"` // Expiration time timestamp
	Nbf float64   `json:"nbf"` // Not before timestamp
	Iat float64   `json:"iat"` // Issued At
	Jti uuid.UUID `json:"jti"` // JWT id claim provides a unique identifier for the JWT.
	// Custom claims
	Ver string   `json:"ver"` // Version
	Cls string   `json:"cls"` // Classification (user or app)
	Rsc string   `json:"rsc"` // Resource (tenantId:tenantName)
	Rol []string `json:"rol"` // Roles (array of strings)
}

func (c Context) GetTenantId() (uuid.UUID, error) {
	parts := strings.Split(c.Rsc, ":")
	if len(parts) < 2 || parts[0] == "" {
		return uuid.Nil, errors.New("rsc field is empty or malformed")
	}
	tenantId, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, fmt.Errorf("error parsing tenant ID: %w", err)
	}
	return tenantId, nil
}

func (c Context) GetTenantName() string {
	return strings.Split(c.Rsc, ":")[1]
}

func (c Context) Repr(input string) string {
	var b strings.Builder
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// clockSkewLeewaySeconds is the tolerance applied to exp/nbf/iat to absorb
// small clock differences between the IdP and this service (contract: ≤ 60s).
const clockSkewLeewaySeconds = 30

func (c Context) Valid() error {
	now := time.Now().Unix()
	// Convert float timestamps to int64 for comparison
	exp := int64(c.Exp)
	nbf := int64(c.Nbf)
	iat := int64(c.Iat)

	// Validate expiration (exp), allowing a small leeway for clock skew.
	if exp != 0 && now > exp+clockSkewLeewaySeconds {
		return errors.New("token has expired")
	}

	// Validate not before (nbf)
	if nbf != 0 && now < nbf-clockSkewLeewaySeconds {
		return errors.New("token not yet valid")
	}

	// Validate issued at (iat)
	if iat != 0 && now < iat-clockSkewLeewaySeconds {
		return errors.New("token issued in the future")
	}

	// NOTE: issuer (`iss`) is enforced per-environment in the auth middleware
	// against Config.Issuer(); it is intentionally not hardcoded here.

	if c.Sub == "" {
		return errors.New("invalid sub")
	}

	// Validate resource (rsc)
	rsc := strings.Split(c.Rsc, ":")
	if len(rsc) != 2 {
		return errors.New("invalid resource")
	}
	_, err := uuid.Parse(rsc[0])
	if err != nil {
		return errors.New("invalid resource")
	}

	return nil
}

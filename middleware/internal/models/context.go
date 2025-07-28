package models

import (
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

func (c Context) GetTenantId() uuid.UUID {
	tenantId, err := uuid.Parse(strings.Split(c.Rsc, ":")[0])
	if err != nil {
		return uuid.Nil
	}
	return tenantId
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

func (c Context) Validate() []ValidationError {
	now := time.Now().Unix()
	// Convert float timestamps to int64 for comparison
	exp := int64(c.Exp)
	nbf := int64(c.Nbf)
	iat := int64(c.Iat)

	// response
	var errors []ValidationError

	// Validate expiration (exp)
	if exp != 0 && now > exp {
		errors = append(errors, ValidationError{
			Field:   "exp",
			Message: "Token has expired.",
		})
	}

	// Validate not before (nbf)
	if nbf != 0 && now < nbf {
		errors = append(errors, ValidationError{
			Field:   "nbf",
			Message: "Token not yet valid.",
		})

	}

	// Validate issed at (iat)
	if iat != 0 && now < iat {
		errors = append(errors, ValidationError{
			Field:   "iat",
			Message: "Token issued in the future.",
		})
	}

	// Validate issuer
	if c.Iss != "https://auth.grasp-daas.com" && c.Iss != "https://auth-dev.grasp-daas.com" {
		errors = append(errors, ValidationError{
			Field:   "iss",
			Message: "Invalid issuer.",
		})
	}

	// Validate subject
	if c.Sub == "" {
		errors = append(errors, ValidationError{
			Field:   "sub",
			Message: "Invalid sub.",
		})
	}

	// Validate resource (rsc)
	rsc := strings.Split(c.Rsc, ":")
	if len(rsc) != 2 {
		errors = append(errors, ValidationError{
			Field:   "rsc",
			Message: "Invalid resource.",
		})
	}
	_, err := uuid.Parse(rsc[0])
	if err != nil {
		errors = append(errors, ValidationError{
			Field:   "rsc",
			Message: "Invalid resource.",
		})
	}

	return errors
}

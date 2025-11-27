package fakes

import (
	"github.com/google/uuid"
	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/claims"
)

func NewTestUserContext(sub string, rsc string) *claims.Context {
	return &claims.Context{
		Sub: sub,
		Jti: uuid.New(),
		Rsc: rsc,
	}
}

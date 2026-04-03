package auth

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	claimsKey contextKey = "auth_claims"
)

type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

type Claims struct {
	Subject        string
	Email          string
	Roles          []Role
	InternalUserID uuid.UUID
}

func (c *Claims) HasRole(r Role) bool {
	for _, role := range c.Roles {
		if role == r {
			return true
		}
	}
	return false
}

func (c *Claims) IsAdmin() bool { return c.HasRole(RoleAdmin) }

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(*Claims)
	return claims, ok
}

func MustClaimsFromContext(ctx context.Context) *Claims {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		panic("auth: claims not found in context — is the auth middleware applied?")
	}
	return claims
}

// Package auth handles JWT validation against Auth0's JWKS endpoint and
// provides typed claim extraction helpers.
//
// Auth0 is used as the external OIDC/OAuth2 provider — this backend NEVER
// issues tokens.  It only validates them.
package auth

import (
	"context"

	"github.com/google/uuid"
)

// contextKey is an unexported type to avoid key collisions in context values.
type contextKey string

const (
	claimsKey contextKey = "auth_claims"
)

// Role represents a user role enforced by RBAC.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// Claims holds the relevant fields extracted from a validated Auth0 JWT.
// The struct is stored in the request context after successful authentication.
type Claims struct {
	// Subject is the Auth0 user ID (e.g. "auth0|abc123" or "google-oauth2|xyz")
	Subject string
	// Email is the user's email address from the token (if present)
	Email string
	// Roles extracted from the custom namespace claim in the token.
	// Auth0 Actions/Rules must inject roles under the configured namespace.
	Roles []Role
	// InternalUserID is the UUID from our own users table, populated by the
	// user-sync middleware that runs after JWT validation.
	InternalUserID uuid.UUID
}

// HasRole reports whether the claims contain the given role.
func (c *Claims) HasRole(r Role) bool {
	for _, role := range c.Roles {
		if role == r {
			return true
		}
	}
	return false
}

// IsAdmin is a convenience helper.
func (c *Claims) IsAdmin() bool { return c.HasRole(RoleAdmin) }

// ── Context helpers ──────────────────────────────────────────────────────────

// WithClaims stores Claims in the context.  Called by the auth middleware
// after successful token validation.
func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// ClaimsFromContext retrieves Claims from the context.
// Returns nil, false if not present (i.e. unauthenticated request).
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(*Claims)
	return claims, ok
}

// MustClaimsFromContext retrieves Claims from context and panics if absent.
// Only use this inside handlers that sit behind the auth middleware.
func MustClaimsFromContext(ctx context.Context) *Claims {
	claims, ok := ClaimsFromContext(ctx)
	if !ok {
		panic("auth: claims not found in context — is the auth middleware applied?")
	}
	return claims
}

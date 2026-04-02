// Package domain contains the User aggregate root and its invariants.
// Users are not created directly by this service — they are synced from Auth0
// on first authenticated request (upsert-on-login pattern).
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Role represents the user's access level within this application.
// Defined here (not in shared/auth) to avoid circular package imports.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// User is the aggregate root for the user domain.
// It mirrors the identity stored in Auth0 and adds application-level fields
// such as the role and creation timestamps.
type User struct {
	// ID is the internal UUID primary key (opaque to the client).
	ID uuid.UUID `db:"id"`
	// ExternalID is the Auth0 subject claim (e.g. "auth0|abc123").
	// This is the stable link between Auth0 and this service.
	ExternalID string `db:"external_id"`
	// Email is sourced from the JWT and kept in sync for convenience.
	Email string `db:"email"`
	// Role controls access level: "user" or "admin".
	Role Role `db:"role"`
	// CreatedAt / UpdatedAt are managed by the database.
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// NewUser creates a new User value ready for persistence.
// The caller provides the Auth0 subject and email from the validated JWT.
func NewUser(externalID, email string) *User {
	return &User{
		ID:         uuid.New(),
		ExternalID: externalID,
		Email:      email,
		Role:       RoleUser, // new users are always standard users
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
}

// Package repository defines the persistence interface and PostgreSQL
// implementation for the User aggregate.
package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/Ammar022/secure-ai-chat-backend/internal/user/domain"
)

// UserRepository defines the contract that any user persistence layer must
// fulfil.  Domain services and controllers depend on this interface, not the
// concrete PostgreSQL implementation, enabling easy testing with mocks.
type UserRepository interface {
	// FindByExternalID returns the user with the given Auth0 subject, or nil.
	FindByExternalID(ctx context.Context, externalID string) (*domain.User, error)
	// FindByID returns the user with the given internal UUID, or nil.
	FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	// Upsert inserts the user if it does not exist, or updates email/role if it does.
	// This is the "upsert on login" pattern — called on every authenticated request.
	Upsert(ctx context.Context, user *domain.User) (*domain.User, error)
	// ListAll returns all users (admin-only operation).
	ListAll(ctx context.Context) ([]*domain.User, error)
}

package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/Ammar022/muhammad-ammar-azam/internal/user/domain"
)

type UserRepository interface {
	FindByExternalID(ctx context.Context, externalID string) (*domain.User, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	Upsert(ctx context.Context, user *domain.User) (*domain.User, error)
	ListAll(ctx context.Context) ([]*domain.User, error)
}

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/Ammar022/secure-ai-chat-backend/internal/user/domain"
)

// postgresUserRepository is the PostgreSQL implementation of UserRepository.
// It uses sqlx for struct scanning and named queries.
type postgresUserRepository struct {
	db *sqlx.DB
}

func NewPostgresUserRepository(db *sqlx.DB) UserRepository {
	return &postgresUserRepository{db: db}
}

func (r *postgresUserRepository) FindByExternalID(ctx context.Context, externalID string) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user,
		`SELECT id, external_id, email, role, created_at, updated_at
		 FROM users
		 WHERE external_id = $1`,
		externalID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // caller checks for nil
		}
		return nil, fmt.Errorf("user repo: find by external_id: %w", err)
	}
	return &user, nil
}

func (r *postgresUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var user domain.User
	err := r.db.GetContext(ctx, &user,
		`SELECT id, external_id, email, role, created_at, updated_at
		 FROM users
		 WHERE id = $1`,
		id,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("user repo: find by id: %w", err)
	}
	return &user, nil
}

func (r *postgresUserRepository) Upsert(ctx context.Context, user *domain.User) (*domain.User, error) {
	if user.ID == (uuid.UUID{}) {
		user.ID = uuid.New()
	}

	var result domain.User
	err := r.db.QueryRowxContext(ctx,
		`INSERT INTO users (id, external_id, email, role, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NOW(), NOW())
		 ON CONFLICT (external_id) DO UPDATE
		   SET email      = EXCLUDED.email,
		       updated_at = NOW()
		 RETURNING id, external_id, email, role, created_at, updated_at`,
		user.ID, user.ExternalID, user.Email, user.Role,
	).StructScan(&result)
	if err != nil {
		return nil, fmt.Errorf("user repo: upsert: %w", err)
	}
	return &result, nil
}

func (r *postgresUserRepository) ListAll(ctx context.Context) ([]*domain.User, error) {
	var users []*domain.User
	err := r.db.SelectContext(ctx, &users,
		`SELECT id, external_id, email, role, created_at, updated_at
		 FROM users
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("user repo: list all: %w", err)
	}
	return users, nil
}

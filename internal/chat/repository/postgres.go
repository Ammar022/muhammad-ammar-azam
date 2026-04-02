// Package repository provides the PostgreSQL implementation of the chat domain
// repository interfaces.  All queries use parameterised placeholders — never
// string concatenation — to prevent SQL injection.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	chatdomain "github.com/Ammar022/secure-ai-chat-backend/internal/chat/domain"
)

// ── ChatRepository ────────────────────────────────────────────────────────────

type postgresChatRepository struct {
	db *sqlx.DB
}

// NewPostgresChatRepository creates the PostgreSQL-backed chat repository.
func NewPostgresChatRepository(db *sqlx.DB) chatdomain.ChatRepository {
	return &postgresChatRepository{db: db}
}

func (r *postgresChatRepository) Create(ctx context.Context, msg *chatdomain.ChatMessage) (*chatdomain.ChatMessage, error) {
	var result chatdomain.ChatMessage
	err := r.db.QueryRowxContext(ctx, `
		INSERT INTO chat_messages (
			id, user_id, subscription_id, question, answer,
			prompt_tokens, completion_tokens, total_tokens,
			response_time_ms, ip_address, request_id, created_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11, NOW()
		)
		RETURNING id, user_id, subscription_id, question, answer,
		          prompt_tokens, completion_tokens, total_tokens,
		          response_time_ms, ip_address, request_id, created_at`,
		msg.ID, msg.UserID, msg.SubscriptionID, msg.Question, msg.Answer,
		msg.PromptTokens, msg.CompletionTokens, msg.TotalTokens,
		msg.ResponseTimeMs, msg.IPAddress, msg.RequestID,
	).StructScan(&result)
	if err != nil {
		return nil, fmt.Errorf("chat repo: create message: %w", err)
	}
	return &result, nil
}

func (r *postgresChatRepository) FindByID(ctx context.Context, id uuid.UUID) (*chatdomain.ChatMessage, error) {
	var msg chatdomain.ChatMessage
	err := r.db.GetContext(ctx, &msg, `
		SELECT id, user_id, subscription_id, question, answer,
		       prompt_tokens, completion_tokens, total_tokens,
		       response_time_ms, ip_address, request_id, created_at
		FROM chat_messages
		WHERE id = $1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("chat repo: find by id: %w", err)
	}
	return &msg, nil
}

func (r *postgresChatRepository) ListByUserID(
	ctx context.Context, userID uuid.UUID, limit, offset int,
) ([]*chatdomain.ChatMessage, int64, error) {
	var total int64
	if err := r.db.GetContext(ctx, &total,
		`SELECT COUNT(*) FROM chat_messages WHERE user_id = $1`, userID); err != nil {
		return nil, 0, fmt.Errorf("chat repo: count messages: %w", err)
	}

	var msgs []*chatdomain.ChatMessage
	err := r.db.SelectContext(ctx, &msgs, `
		SELECT id, user_id, subscription_id, question, answer,
		       prompt_tokens, completion_tokens, total_tokens,
		       response_time_ms, ip_address, request_id, created_at
		FROM chat_messages
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("chat repo: list messages: %w", err)
	}
	return msgs, total, nil
}

// ── QuotaRepository ───────────────────────────────────────────────────────────

type postgresQuotaRepository struct {
	db *sqlx.DB
}

// NewPostgresQuotaRepository creates the PostgreSQL-backed quota repository.
func NewPostgresQuotaRepository(db *sqlx.DB) chatdomain.QuotaRepository {
	return &postgresQuotaRepository{db: db}
}

// GetOrCreateForMonth fetches the quota record inside a transaction.
// The SELECT … FOR UPDATE acquires a row-level lock, preventing two concurrent
// requests from both reading "0 used" and both successfully claiming a slot.
func (r *postgresQuotaRepository) GetOrCreateForMonth(
	ctx context.Context, tx *sqlx.Tx, userID uuid.UUID, month string,
) (*chatdomain.QuotaUsage, error) {
	// Upsert: insert a zero-count row if this is the first request this month.
	// The RETURNING clause means we always get the current row back in one query.
	var quota chatdomain.QuotaUsage
	err := tx.QueryRowxContext(ctx, `
		INSERT INTO quota_usages (id, user_id, month, free_messages_used, created_at, updated_at)
		VALUES ($1, $2, $3, 0, NOW(), NOW())
		ON CONFLICT (user_id, month) DO UPDATE SET updated_at = quota_usages.updated_at
		RETURNING id, user_id, month, free_messages_used, created_at, updated_at`,
		uuid.New(), userID, month,
	).StructScan(&quota)
	if err != nil {
		return nil, fmt.Errorf("quota repo: get or create: %w", err)
	}

	// Lock the row so concurrent transactions must wait
	var locked chatdomain.QuotaUsage
	err = tx.QueryRowxContext(ctx, `
		SELECT id, user_id, month, free_messages_used, created_at, updated_at
		FROM quota_usages
		WHERE user_id = $1 AND month = $2
		FOR UPDATE`, userID, month,
	).StructScan(&locked)
	if err != nil {
		return nil, fmt.Errorf("quota repo: lock row: %w", err)
	}
	return &locked, nil
}

// IncrementFreeUsage atomically bumps the free_messages_used counter.
func (r *postgresQuotaRepository) IncrementFreeUsage(
	ctx context.Context, tx *sqlx.Tx, id uuid.UUID,
) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE quota_usages
		SET free_messages_used = free_messages_used + 1,
		    updated_at = NOW()
		WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("quota repo: increment free usage: %w", err)
	}
	return nil
}

// Package repository provides the PostgreSQL implementation of the
// subscription domain repository interfaces.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	chatdomain "github.com/Ammar022/secure-ai-chat-backend/internal/chat/domain"
	subdomain "github.com/Ammar022/secure-ai-chat-backend/internal/subscription/domain"
)

// postgresSubscriptionRepository implements SubscriptionRepository.
type postgresSubscriptionRepository struct {
	db *sqlx.DB
}

// NewPostgresSubscriptionRepository creates the PostgreSQL-backed subscription repository.
func NewPostgresSubscriptionRepository(db *sqlx.DB) subdomain.SubscriptionRepository {
	return &postgresSubscriptionRepository{db: db}
}

func (r *postgresSubscriptionRepository) Create(ctx context.Context, sub *subdomain.Subscription) (*subdomain.Subscription, error) {
	var result subdomain.Subscription
	err := r.db.QueryRowxContext(ctx, `
		INSERT INTO subscriptions (
			id, user_id, tier, billing_cycle, auto_renew,
			max_messages, messages_used, price,
			start_date, end_date, renewal_date,
			is_active, cancelled_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11,
			$12, $13, NOW(), NOW()
		)
		RETURNING id, user_id, tier, billing_cycle, auto_renew,
		          max_messages, messages_used, price,
		          start_date, end_date, renewal_date,
		          is_active, cancelled_at, created_at, updated_at`,
		sub.ID, sub.UserID, sub.Tier, sub.BillingCycle, sub.AutoRenew,
		sub.MaxMessages, sub.MessagesUsed, sub.Price,
		sub.StartDate, sub.EndDate, sub.RenewalDate,
		sub.IsActive, sub.CancelledAt,
	).StructScan(&result)
	if err != nil {
		return nil, fmt.Errorf("subscription repo: create: %w", err)
	}
	return &result, nil
}

func (r *postgresSubscriptionRepository) FindByID(ctx context.Context, id uuid.UUID) (*subdomain.Subscription, error) {
	var sub subdomain.Subscription
	err := r.db.GetContext(ctx, &sub, `
		SELECT id, user_id, tier, billing_cycle, auto_renew,
		       max_messages, messages_used, price,
		       start_date, end_date, renewal_date,
		       is_active, cancelled_at, created_at, updated_at
		FROM subscriptions WHERE id = $1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("subscription repo: find by id: %w", err)
	}
	return &sub, nil
}

func (r *postgresSubscriptionRepository) FindByUserID(ctx context.Context, userID uuid.UUID) ([]*subdomain.Subscription, error) {
	var subs []*subdomain.Subscription
	err := r.db.SelectContext(ctx, &subs, `
		SELECT id, user_id, tier, billing_cycle, auto_renew,
		       max_messages, messages_used, price,
		       start_date, end_date, renewal_date,
		       is_active, cancelled_at, created_at, updated_at
		FROM subscriptions
		WHERE user_id = $1
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("subscription repo: find by user: %w", err)
	}
	return subs, nil
}

func (r *postgresSubscriptionRepository) Update(ctx context.Context, sub *subdomain.Subscription) (*subdomain.Subscription, error) {
	var result subdomain.Subscription
	err := r.db.QueryRowxContext(ctx, `
		UPDATE subscriptions SET
			auto_renew    = $2,
			messages_used = $3,
			start_date    = $4,
			end_date      = $5,
			renewal_date  = $6,
			is_active     = $7,
			cancelled_at  = $8,
			updated_at    = NOW()
		WHERE id = $1
		RETURNING id, user_id, tier, billing_cycle, auto_renew,
		          max_messages, messages_used, price,
		          start_date, end_date, renewal_date,
		          is_active, cancelled_at, created_at, updated_at`,
		sub.ID,
		sub.AutoRenew, sub.MessagesUsed,
		sub.StartDate, sub.EndDate, sub.RenewalDate,
		sub.IsActive, sub.CancelledAt,
	).StructScan(&result)
	if err != nil {
		return nil, fmt.Errorf("subscription repo: update: %w", err)
	}
	return &result, nil
}

func (r *postgresSubscriptionRepository) FindDueForRenewal(ctx context.Context) ([]*subdomain.Subscription, error) {
	var subs []*subdomain.Subscription
	err := r.db.SelectContext(ctx, &subs, `
		SELECT id, user_id, tier, billing_cycle, auto_renew,
		       max_messages, messages_used, price,
		       start_date, end_date, renewal_date,
		       is_active, cancelled_at, created_at, updated_at
		FROM subscriptions
		WHERE auto_renew = TRUE
		  AND is_active  = TRUE
		  AND cancelled_at IS NULL
		  AND renewal_date <= $1`,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("subscription repo: find due renewals: %w", err)
	}
	return subs, nil
}

// ── SubscriptionQuotaRepository ──────────────────────────────────────────────
// Implements chatdomain.SubscriptionQuotaRepository — the cross-module
// contract that allows the chat quota engine to deduct from subscriptions
// without creating a circular package dependency.

type postgresSubscriptionQuotaRepository struct {
	db *sqlx.DB
}

// NewPostgresSubscriptionQuotaRepository creates the quota-side subscription repository.
func NewPostgresSubscriptionQuotaRepository(db *sqlx.DB) chatdomain.SubscriptionQuotaRepository {
	return &postgresSubscriptionQuotaRepository{db: db}
}

// FindActiveForUserOrderedByCreatedDesc returns active subscriptions that
// either have remaining capacity or are enterprise (unlimited).  The results
// are ordered newest-first so the chat service charges the most-recently
// purchased bundle first.
func (r *postgresSubscriptionQuotaRepository) FindActiveForUserOrderedByCreatedDesc(
	ctx context.Context, tx *sqlx.Tx, userID uuid.UUID,
) ([]*chatdomain.SubscriptionForQuota, error) {
	rows, err := tx.QueryxContext(ctx, `
		SELECT id, tier, messages_used, max_messages, is_active
		FROM subscriptions
		WHERE user_id    = $1
		  AND is_active  = TRUE
		  AND cancelled_at IS NULL
		  AND end_date   > NOW()
		ORDER BY created_at DESC
		FOR UPDATE`, userID)
	if err != nil {
		return nil, fmt.Errorf("sub quota repo: find active: %w", err)
	}
	defer rows.Close()

	var results []*chatdomain.SubscriptionForQuota
	for rows.Next() {
		var s chatdomain.SubscriptionForQuota
		if err := rows.Scan(&s.ID, &s.Tier, &s.MessagesUsed, &s.MaxMessages, &s.IsActive); err != nil {
			return nil, fmt.Errorf("sub quota repo: scan: %w", err)
		}
		results = append(results, &s)
	}
	return results, rows.Err()
}

// DeductMessage atomically increments messages_used for a subscription.
func (r *postgresSubscriptionQuotaRepository) DeductMessage(
	ctx context.Context, tx *sqlx.Tx, subscriptionID uuid.UUID,
) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE subscriptions
		SET messages_used = messages_used + 1,
		    updated_at    = NOW()
		WHERE id = $1`, subscriptionID)
	if err != nil {
		return fmt.Errorf("sub quota repo: deduct message: %w", err)
	}
	return nil
}

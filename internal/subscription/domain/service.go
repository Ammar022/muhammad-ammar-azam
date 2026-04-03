package domain

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	apperrors "github.com/Ammar022/muhammad-ammar-azam/internal/shared/errors"
)

// SubscriptionRepository defines the persistence contract for subscriptions.
// Defined here (in the domain) so the domain has no dependency on infrastructure.
type SubscriptionRepository interface {
	Create(ctx context.Context, sub *Subscription) (*Subscription, error)
	FindByID(ctx context.Context, id uuid.UUID) (*Subscription, error)
	FindByUserID(ctx context.Context, userID uuid.UUID) ([]*Subscription, error)
	Update(ctx context.Context, sub *Subscription) (*Subscription, error)
	// FindDueForRenewal returns active subscriptions with auto_renew=true
	// whose renewal_date is on or before now.
	FindDueForRenewal(ctx context.Context) ([]*Subscription, error)
}

// SubscriptionService orchestrates subscription lifecycle operations:
// creation, cancellation, renewal simulation, and auto-renew toggling.
type SubscriptionService struct {
	repo   SubscriptionRepository
	policy *SubscriptionPolicy
}

// NewSubscriptionService creates a SubscriptionService.
func NewSubscriptionService(repo SubscriptionRepository) *SubscriptionService {
	return &SubscriptionService{
		repo:   repo,
		policy: NewSubscriptionPolicy(),
	}
}

// CreateSubscription creates a new subscription after enforcing domain policy.
// Users may hold multiple active subscription bundles simultaneously; the
// quota engine always deducts from the bundle with the most recent remaining
// quota (FindActiveForUserOrderedByCreatedDesc).
func (s *SubscriptionService) CreateSubscription(
	ctx context.Context,
	requestingUserID uuid.UUID,
	tier Tier,
	cycle BillingCycle,
	autoRenew bool,
) (*Subscription, error) {
	if err := s.policy.CanCreate(requestingUserID, requestingUserID); err != nil {
		return nil, err
	}

	sub, err := NewSubscription(requestingUserID, tier, cycle, autoRenew)
	if err != nil {
		return nil, apperrors.Wrap(400, "INVALID_SUBSCRIPTION", err.Error(), err)
	}

	saved, err := s.repo.Create(ctx, sub)
	if err != nil {
		return nil, fmt.Errorf("subscription service: create: %w", err)
	}

	log.Ctx(ctx).Info().
		Str("subscription_id", saved.ID.String()).
		Str("user_id", requestingUserID.String()).
		Str("tier", string(tier)).
		Msg("subscription: created")

	return saved, nil
}

// GetSubscription returns a subscription, enforcing ownership.
func (s *SubscriptionService) GetSubscription(
	ctx context.Context, requestingUserID, subscriptionID uuid.UUID,
) (*Subscription, error) {
	sub, err := s.repo.FindByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, apperrors.ErrNotFound
	}
	if err := s.policy.CanView(requestingUserID, sub.UserID); err != nil {
		return nil, err
	}
	return sub, nil
}

// ListSubscriptions returns all subscriptions belonging to the requesting user.
func (s *SubscriptionService) ListSubscriptions(
	ctx context.Context, userID uuid.UUID,
) ([]*Subscription, error) {
	return s.repo.FindByUserID(ctx, userID)
}

// CancelSubscription cancels a subscription.
// The subscription remains active until the end of the billing cycle
// (access preserved, renewal prevented).
func (s *SubscriptionService) CancelSubscription(
	ctx context.Context, requestingUserID, subscriptionID uuid.UUID,
) (*Subscription, error) {
	sub, err := s.repo.FindByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, apperrors.ErrNotFound
	}
	if err := s.policy.CanCancel(requestingUserID, sub.UserID); err != nil {
		return nil, err
	}

	if err := sub.Cancel(); err != nil {
		return nil, apperrors.Wrap(400, "CANCEL_FAILED", err.Error(), err)
	}

	updated, err := s.repo.Update(ctx, sub)
	if err != nil {
		return nil, fmt.Errorf("subscription service: cancel update: %w", err)
	}

	log.Ctx(ctx).Info().
		Str("subscription_id", subscriptionID.String()).
		Str("user_id", requestingUserID.String()).
		Msg("subscription: cancelled")

	return updated, nil
}

// ToggleAutoRenew enables or disables auto-renew for a subscription.
func (s *SubscriptionService) ToggleAutoRenew(
	ctx context.Context, requestingUserID, subscriptionID uuid.UUID, enable bool,
) (*Subscription, error) {
	sub, err := s.repo.FindByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, apperrors.ErrNotFound
	}
	if err := s.policy.CanToggleAutoRenew(requestingUserID, sub.UserID); err != nil {
		return nil, err
	}
	if sub.CancelledAt != nil {
		return nil, apperrors.ErrSubscriptionCancelled
	}

	sub.AutoRenew = enable

	updated, err := s.repo.Update(ctx, sub)
	if err != nil {
		return nil, fmt.Errorf("subscription service: toggle auto-renew: %w", err)
	}
	return updated, nil
}

// ProcessRenewals is called by a background job (or can be triggered via admin
// endpoint).  It finds all subscriptions due for renewal and:
//   - Simulates a payment attempt (30% failure rate)
//   - On success: calls Subscription.Renew() and persists
//   - On failure: calls Subscription.Deactivate() and persists
//
// Historical usage data is preserved in both cases.
func (s *SubscriptionService) ProcessRenewals(ctx context.Context) error {
	due, err := s.repo.FindDueForRenewal(ctx)
	if err != nil {
		return fmt.Errorf("subscription service: find due renewals: %w", err)
	}

	for _, sub := range due {
		if err := s.renewOne(ctx, sub); err != nil {
			log.Ctx(ctx).Error().Err(err).
				Str("subscription_id", sub.ID.String()).
				Msg("subscription: renewal processing error")
		}
	}
	return nil
}

// renewOne processes a single subscription renewal.
func (s *SubscriptionService) renewOne(ctx context.Context, sub *Subscription) error {
	// Simulate payment processing: 30% failure probability
	paymentSucceeded := simulatePayment()

	if !paymentSucceeded {
		log.Ctx(ctx).Warn().
			Str("subscription_id", sub.ID.String()).
			Str("user_id", sub.UserID.String()).
			Msg("subscription: payment failed, deactivating")

		sub.Deactivate("payment_failed")
		_, err := s.repo.Update(ctx, sub)
		return err
	}

	if err := sub.Renew(); err != nil {
		return fmt.Errorf("subscription: renew aggregate: %w", err)
	}

	_, err := s.repo.Update(ctx, sub)
	if err != nil {
		return fmt.Errorf("subscription: persist renewal: %w", err)
	}

	log.Ctx(ctx).Info().
		Str("subscription_id", sub.ID.String()).
		Str("user_id", sub.UserID.String()).
		Time("new_end_date", sub.EndDate).
		Msg("subscription: renewed successfully")

	return nil
}

// simulatePayment returns true (success) ~70% of the time to simulate real
// payment gateway behaviour where failures occasionally occur.
func simulatePayment() bool {
	return rand.Float32() > 0.30
}

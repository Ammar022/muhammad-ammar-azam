package domain

import (
	"github.com/google/uuid"

	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
)

// SubscriptionPolicy enforces domain-level access rules for subscriptions.
// It is the second authorization layer (RBAC at the controller level is first).
// Pure functions with no I/O — trivially testable.
type SubscriptionPolicy struct{}

// NewSubscriptionPolicy creates a SubscriptionPolicy.
func NewSubscriptionPolicy() *SubscriptionPolicy { return &SubscriptionPolicy{} }

// CanCreate verifies a user may create a subscription for themselves.
// In the current model users may only create subscriptions for their own account.
func (p *SubscriptionPolicy) CanCreate(requestingUserID, targetUserID uuid.UUID) error {
	if requestingUserID != targetUserID {
		return apperrors.ErrForbidden
	}
	return nil
}

// CanView verifies a user may view a given subscription.
func (p *SubscriptionPolicy) CanView(requestingUserID, subscriptionOwnerID uuid.UUID) error {
	if requestingUserID != subscriptionOwnerID {
		return apperrors.ErrForbidden
	}
	return nil
}

// CanCancel verifies a user may cancel a given subscription.
// Cancelling someone else's subscription is forbidden; cancelling an already
// cancelled subscription is caught by the aggregate's Cancel() method.
func (p *SubscriptionPolicy) CanCancel(requestingUserID, subscriptionOwnerID uuid.UUID) error {
	if requestingUserID != subscriptionOwnerID {
		return apperrors.ErrForbidden
	}
	return nil
}

// CanToggleAutoRenew verifies the requesting user owns the subscription.
func (p *SubscriptionPolicy) CanToggleAutoRenew(requestingUserID, subscriptionOwnerID uuid.UUID) error {
	if requestingUserID != subscriptionOwnerID {
		return apperrors.ErrForbidden
	}
	return nil
}

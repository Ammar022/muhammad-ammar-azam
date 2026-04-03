package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Tier represents the subscription plan level
type Tier string

const (
	TierBasic      Tier = "basic"      // 10 messages
	TierPro        Tier = "pro"        // 100 messages
	TierEnterprise Tier = "enterprise" // unlimited
)

// MaxMessages returns the message cap for a tier.
// Enterprise returns -1 to signal "unlimited".
func (t Tier) MaxMessages() int {
	switch t {
	case TierBasic:
		return 10
	case TierPro:
		return 100
	case TierEnterprise:
		return -1 // unlimited
	default:
		return 0
	}
}

// Price returns the monthly base price for a tier in USD (as cents).
func (t Tier) Price() float64 {
	switch t {
	case TierBasic:
		return 9.99
	case TierPro:
		return 49.99
	case TierEnterprise:
		return 199.99
	default:
		return 0
	}
}

// BillingCycle defines how often the subscription renews.
type BillingCycle string

const (
	BillingMonthly BillingCycle = "monthly"
	BillingYearly  BillingCycle = "yearly"
)

// Subscription is the aggregate root for the subscription domain
// All state transitions are mediated through methods to preserve invariants
type Subscription struct {
	ID           uuid.UUID    `db:"id"`
	UserID       uuid.UUID    `db:"user_id"`
	Tier         Tier         `db:"tier"`
	BillingCycle BillingCycle `db:"billing_cycle"`
	AutoRenew    bool         `db:"auto_renew"`

	MaxMessages  int `db:"max_messages"`
	MessagesUsed int `db:"messages_used"`

	Price       float64   `db:"price"`
	StartDate   time.Time `db:"start_date"`
	EndDate     time.Time `db:"end_date"`
	RenewalDate time.Time `db:"renewal_date"`
	IsActive    bool      `db:"is_active"`

	CancelledAt *time.Time `db:"cancelled_at"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

// NewSubscription creates a new Subscription value enforcing all creation
// invariants.  The billing price for yearly is discounted by 20%
func NewSubscription(userID uuid.UUID, tier Tier, cycle BillingCycle, autoRenew bool) (*Subscription, error) {
	if tier != TierBasic && tier != TierPro && tier != TierEnterprise {
		return nil, errors.New("subscription: invalid tier")
	}
	if cycle != BillingMonthly && cycle != BillingYearly {
		return nil, errors.New("subscription: invalid billing cycle")
	}

	now := time.Now().UTC()
	endDate := calculateEndDate(now, cycle)
	renewalDate := endDate // renewal is triggered at end of current cycle

	price := tier.Price()
	if cycle == BillingYearly {
		price = price * 12 * 0.80 // 20% annual discount
	}

	return &Subscription{
		ID:           uuid.New(),
		UserID:       userID,
		Tier:         tier,
		BillingCycle: cycle,
		AutoRenew:    autoRenew,
		MaxMessages:  tier.MaxMessages(),
		MessagesUsed: 0,
		Price:        price,
		StartDate:    now,
		EndDate:      endDate,
		RenewalDate:  renewalDate,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// Cancel marks the subscription as cancelled.  Calling this:
//   - Sets CancelledAt to now
//   - Disables AutoRenew
//   - Keeps IsActive = true until the end of the billing cycle
//     (so the user retains access until their paid period ends)
//
// Invariant: a cancelled subscription cannot be cancelled again.
func (s *Subscription) Cancel() error {
	if s.CancelledAt != nil {
		return errors.New("subscription: already cancelled")
	}
	now := time.Now().UTC()
	s.CancelledAt = &now
	s.AutoRenew = false
	s.UpdatedAt = now
	return nil
}

// Renew extends the subscription by one billing cycle and resets MessagesUsed.
// Returns an error if the subscription is cancelled or already inactive.
func (s *Subscription) Renew() error {
	if s.CancelledAt != nil {
		return errors.New("subscription: cannot renew a cancelled subscription")
	}
	if !s.AutoRenew {
		return errors.New("subscription: auto-renew is disabled")
	}

	now := time.Now().UTC()
	newEnd := calculateEndDate(s.EndDate, s.BillingCycle)
	s.StartDate = s.EndDate
	s.EndDate = newEnd
	s.RenewalDate = newEnd
	s.MessagesUsed = 0
	s.IsActive = true
	s.UpdatedAt = now
	return nil
}

// Deactivate marks the subscription as inactive (e.g. after payment failure).
func (s *Subscription) Deactivate(reason string) {
	s.IsActive = false
	s.UpdatedAt = time.Now().UTC()
}

// HasCapacity reports whether the subscription still has message quota.
// Enterprise subscriptions always have capacity.
func (s *Subscription) HasCapacity() bool {
	if !s.IsActive {
		return false
	}
	if s.MaxMessages == -1 {
		return true // enterprise = unlimited
	}
	return s.MessagesUsed < s.MaxMessages
}

// RemainingMessages returns the remaining message quota.
// Returns -1 for enterprise (unlimited).
func (s *Subscription) RemainingMessages() int {
	if s.MaxMessages == -1 {
		return -1
	}
	remaining := s.MaxMessages - s.MessagesUsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IsValidTier checks if the tier string is valid.
func IsValidTier(tier string) bool {
	switch Tier(tier) {
	case TierBasic, TierPro, TierEnterprise:
		return true
	default:
		return false
	}
}

// IsValidBillingCycle checks if the billing cycle string is valid.
func IsValidBillingCycle(cycle string) bool {
	switch BillingCycle(cycle) {
	case BillingMonthly, BillingYearly:
		return true
	default:
		return false
	}
}

// calculateEndDate returns the end-of-period date from a given start point.
func calculateEndDate(from time.Time, cycle BillingCycle) time.Time {
	switch cycle {
	case BillingYearly:
		return from.AddDate(1, 0, 0)
	default: // monthly
		return from.AddDate(0, 1, 0)
	}
}

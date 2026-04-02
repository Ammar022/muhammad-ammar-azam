// Package unit contains pure domain logic tests.  These tests have zero
// infrastructure dependencies — no database, no HTTP server, no Auth0.
// They run fast and are the first line of defence against regressions.
package unit

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	subdomain "github.com/Ammar022/secure-ai-chat-backend/internal/subscription/domain"
)

// ── Tier tests ────────────────────────────────────────────────────────────────

func TestTier_MaxMessages(t *testing.T) {
	cases := []struct {
		tier     subdomain.Tier
		expected int
	}{
		{subdomain.TierBasic, 10},
		{subdomain.TierPro, 100},
		{subdomain.TierEnterprise, -1}, // -1 = unlimited
	}
	for _, tc := range cases {
		t.Run(string(tc.tier), func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.tier.MaxMessages())
		})
	}
}

func TestTier_Price(t *testing.T) {
	assert.Equal(t, 9.99, subdomain.TierBasic.Price())
	assert.Equal(t, 49.99, subdomain.TierPro.Price())
	assert.Equal(t, 199.99, subdomain.TierEnterprise.Price())
}

// ── NewSubscription tests ─────────────────────────────────────────────────────

func TestNewSubscription_Monthly(t *testing.T) {
	userID := uuid.New()
	sub, err := subdomain.NewSubscription(userID, subdomain.TierBasic, subdomain.BillingMonthly, true)

	require.NoError(t, err)
	assert.Equal(t, userID, sub.UserID)
	assert.Equal(t, subdomain.TierBasic, sub.Tier)
	assert.Equal(t, subdomain.BillingMonthly, sub.BillingCycle)
	assert.True(t, sub.AutoRenew)
	assert.Equal(t, 10, sub.MaxMessages)
	assert.Equal(t, 0, sub.MessagesUsed)
	assert.True(t, sub.IsActive)
	assert.Nil(t, sub.CancelledAt)

	// Monthly: end date should be ~1 month after start
	expectedEnd := sub.StartDate.AddDate(0, 1, 0)
	assert.WithinDuration(t, expectedEnd, sub.EndDate, time.Second)
}

func TestNewSubscription_Yearly_PriceDiscount(t *testing.T) {
	userID := uuid.New()
	sub, err := subdomain.NewSubscription(userID, subdomain.TierPro, subdomain.BillingYearly, false)

	require.NoError(t, err)
	// Yearly price = monthly × 12 × 0.80 = 49.99 × 12 × 0.80
	expectedPrice := 49.99 * 12 * 0.80
	assert.InDelta(t, expectedPrice, sub.Price, 0.01)

	// Yearly: end date should be ~1 year after start
	expectedEnd := sub.StartDate.AddDate(1, 0, 0)
	assert.WithinDuration(t, expectedEnd, sub.EndDate, time.Second)
}

func TestNewSubscription_InvalidTier(t *testing.T) {
	userID := uuid.New()
	_, err := subdomain.NewSubscription(userID, subdomain.Tier("invalid"), subdomain.BillingMonthly, true)
	assert.Error(t, err)
}

func TestNewSubscription_InvalidCycle(t *testing.T) {
	userID := uuid.New()
	_, err := subdomain.NewSubscription(userID, subdomain.TierBasic, subdomain.BillingCycle("weekly"), true)
	assert.Error(t, err)
}

// ── Subscription.Cancel tests ─────────────────────────────────────────────────

func TestSubscription_Cancel(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)

	err := sub.Cancel()
	require.NoError(t, err)

	assert.NotNil(t, sub.CancelledAt)
	assert.False(t, sub.AutoRenew, "auto_renew must be disabled on cancel")
	// IsActive stays TRUE until end of billing cycle
	assert.True(t, sub.IsActive, "subscription remains active until billing period ends")
}

func TestSubscription_CancelTwice_ReturnsError(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	require.NoError(t, sub.Cancel())

	// Second cancel must fail
	err := sub.Cancel()
	assert.Error(t, err)
}

// ── Subscription.Renew tests ──────────────────────────────────────────────────

func TestSubscription_Renew_ResetsUsage(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierPro)
	sub.MessagesUsed = 50 // simulate partial usage

	err := sub.Renew()
	require.NoError(t, err)

	assert.Equal(t, 0, sub.MessagesUsed, "messages_used must reset to 0 on renewal")
	assert.True(t, sub.IsActive)
}

func TestSubscription_Renew_ExtendsBillingPeriod(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierPro)
	originalEnd := sub.EndDate

	err := sub.Renew()
	require.NoError(t, err)

	// New start = old end; new end = old end + 1 month
	assert.WithinDuration(t, originalEnd, sub.StartDate, time.Second)
	expectedNewEnd := originalEnd.AddDate(0, 1, 0)
	assert.WithinDuration(t, expectedNewEnd, sub.EndDate, time.Second)
}

func TestSubscription_Renew_CancelledSubscription_Fails(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierPro)
	require.NoError(t, sub.Cancel())

	err := sub.Renew()
	assert.Error(t, err)
}

func TestSubscription_Renew_AutoRenewDisabled_Fails(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierPro)
	sub.AutoRenew = false

	err := sub.Renew()
	assert.Error(t, err)
}

// ── Subscription.HasCapacity tests ───────────────────────────────────────────

func TestSubscription_HasCapacity(t *testing.T) {
	cases := []struct {
		name         string
		tier         subdomain.Tier
		messagesUsed int
		isActive     bool
		expected     bool
	}{
		{"basic with capacity", subdomain.TierBasic, 5, true, true},
		{"basic at limit", subdomain.TierBasic, 10, true, false},
		{"basic over limit", subdomain.TierBasic, 11, true, false},
		{"enterprise always has capacity", subdomain.TierEnterprise, 99999, true, true},
		{"inactive subscription", subdomain.TierPro, 0, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := newTestSubscription(t, tc.tier)
			sub.MessagesUsed = tc.messagesUsed
			sub.IsActive = tc.isActive
			if tc.tier == subdomain.TierEnterprise {
				sub.MaxMessages = -1
			}
			assert.Equal(t, tc.expected, sub.HasCapacity())
		})
	}
}

// ── Subscription.RemainingMessages tests ─────────────────────────────────────

func TestSubscription_RemainingMessages(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	sub.MessagesUsed = 3
	assert.Equal(t, 7, sub.RemainingMessages())

	// Enterprise always returns -1
	enterprise := newTestSubscription(t, subdomain.TierEnterprise)
	enterprise.MaxMessages = -1
	assert.Equal(t, -1, enterprise.RemainingMessages())
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func newTestSubscription(t *testing.T, tier subdomain.Tier) *subdomain.Subscription {
	t.Helper()
	sub, err := subdomain.NewSubscription(uuid.New(), tier, subdomain.BillingMonthly, true)
	require.NoError(t, err)
	return sub
}

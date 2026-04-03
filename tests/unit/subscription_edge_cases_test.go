package unit

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	subdomain "github.com/Ammar022/secure-ai-chat-backend/internal/subscription/domain"
)

// ── Tier edge cases ───────────────────────────────────────────────────────────

func TestTier_MaxMessages_UnknownTierReturnsZero(t *testing.T) {
	assert.Equal(t, 0, subdomain.Tier("unknown").MaxMessages())
	assert.Equal(t, 0, subdomain.Tier("").MaxMessages())
	assert.Equal(t, 0, subdomain.Tier("BASIC").MaxMessages()) // case-sensitive
}

func TestTier_Price_UnknownTierReturnsZero(t *testing.T) {
	assert.Equal(t, 0.0, subdomain.Tier("unknown").Price())
	assert.Equal(t, 0.0, subdomain.Tier("").Price())
}

// ── IsValidTier / IsValidBillingCycle ─────────────────────────────────────────

func TestIsValidTier(t *testing.T) {
	assert.True(t, subdomain.IsValidTier("basic"))
	assert.True(t, subdomain.IsValidTier("pro"))
	assert.True(t, subdomain.IsValidTier("enterprise"))
	assert.False(t, subdomain.IsValidTier(""))
	assert.False(t, subdomain.IsValidTier("BASIC"))
	assert.False(t, subdomain.IsValidTier("free"))
	assert.False(t, subdomain.IsValidTier("premium"))
}

func TestIsValidBillingCycle(t *testing.T) {
	assert.True(t, subdomain.IsValidBillingCycle("monthly"))
	assert.True(t, subdomain.IsValidBillingCycle("yearly"))
	assert.False(t, subdomain.IsValidBillingCycle(""))
	assert.False(t, subdomain.IsValidBillingCycle("weekly"))
	assert.False(t, subdomain.IsValidBillingCycle("MONTHLY"))
	assert.False(t, subdomain.IsValidBillingCycle("annual"))
}

// ── NewSubscription edge cases ────────────────────────────────────────────────

func TestNewSubscription_HasNonZeroUUID(t *testing.T) {
	sub, err := subdomain.NewSubscription(uuid.New(), subdomain.TierBasic, subdomain.BillingMonthly, true)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.UUID{}, sub.ID, "subscription ID must be a new UUID, not zero")
}

func TestNewSubscription_AutoRenewFalse(t *testing.T) {
	sub, err := subdomain.NewSubscription(uuid.New(), subdomain.TierBasic, subdomain.BillingMonthly, false)
	require.NoError(t, err)
	assert.False(t, sub.AutoRenew)
}

func TestNewSubscription_ZeroMessagesUsed(t *testing.T) {
	sub, err := subdomain.NewSubscription(uuid.New(), subdomain.TierPro, subdomain.BillingMonthly, true)
	require.NoError(t, err)
	assert.Equal(t, 0, sub.MessagesUsed)
}

func TestNewSubscription_EnterpriseYearly_PriceDiscount(t *testing.T) {
	sub, err := subdomain.NewSubscription(uuid.New(), subdomain.TierEnterprise, subdomain.BillingYearly, true)
	require.NoError(t, err)
	expectedPrice := 199.99 * 12 * 0.80
	assert.InDelta(t, expectedPrice, sub.Price, 0.01)
}

func TestNewSubscription_BasicYearly_PriceDiscount(t *testing.T) {
	sub, err := subdomain.NewSubscription(uuid.New(), subdomain.TierBasic, subdomain.BillingYearly, true)
	require.NoError(t, err)
	expectedPrice := 9.99 * 12 * 0.80
	assert.InDelta(t, expectedPrice, sub.Price, 0.01)
}

func TestNewSubscription_Yearly_EndDateIsOneYearOut(t *testing.T) {
	sub, err := subdomain.NewSubscription(uuid.New(), subdomain.TierPro, subdomain.BillingYearly, true)
	require.NoError(t, err)
	expected := sub.StartDate.AddDate(1, 0, 0)
	assert.WithinDuration(t, expected, sub.EndDate, time.Second)
}

func TestNewSubscription_RenewalDateEqualsEndDate(t *testing.T) {
	sub, err := subdomain.NewSubscription(uuid.New(), subdomain.TierBasic, subdomain.BillingMonthly, true)
	require.NoError(t, err)
	assert.Equal(t, sub.EndDate, sub.RenewalDate)
}

func TestNewSubscription_MaxMessagesCachedFromTier(t *testing.T) {
	cases := []struct {
		tier     subdomain.Tier
		expected int
	}{
		{subdomain.TierBasic, 10},
		{subdomain.TierPro, 100},
		{subdomain.TierEnterprise, -1},
	}
	for _, tc := range cases {
		sub, err := subdomain.NewSubscription(uuid.New(), tc.tier, subdomain.BillingMonthly, true)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, sub.MaxMessages, "tier %s", tc.tier)
	}
}

// ── Cancel edge cases ─────────────────────────────────────────────────────────

func TestSubscription_Cancel_SetsUpdatedat(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	before := time.Now().UTC().Add(-time.Second)

	err := sub.Cancel()
	require.NoError(t, err)
	assert.True(t, sub.UpdatedAt.After(before), "UpdatedAt must be refreshed on cancel")
}

func TestSubscription_Cancel_DisablesAutoRenew(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	sub.AutoRenew = true

	require.NoError(t, sub.Cancel())
	assert.False(t, sub.AutoRenew, "auto_renew must be disabled after cancel")
}

func TestSubscription_Cancel_IsActiveRemainsTrue(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	require.NoError(t, sub.Cancel())
	assert.True(t, sub.IsActive, "subscription remains active until billing cycle ends")
}

func TestSubscription_Cancel_SetsCancelledAtTimestamp(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	before := time.Now().UTC().Add(-time.Second)

	require.NoError(t, sub.Cancel())
	require.NotNil(t, sub.CancelledAt)
	assert.True(t, sub.CancelledAt.After(before), "CancelledAt must be a recent timestamp")
}

// ── Renew edge cases ──────────────────────────────────────────────────────────

func TestSubscription_Renew_Yearly_ExtendsByOneYear(t *testing.T) {
	sub, err := subdomain.NewSubscription(uuid.New(), subdomain.TierPro, subdomain.BillingYearly, true)
	require.NoError(t, err)
	originalEnd := sub.EndDate

	require.NoError(t, sub.Renew())

	expectedNewEnd := originalEnd.AddDate(1, 0, 0)
	assert.WithinDuration(t, expectedNewEnd, sub.EndDate, time.Second)
}

func TestSubscription_Renew_SetsNewRenewalDate(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	require.NoError(t, sub.Renew())
	assert.Equal(t, sub.EndDate, sub.RenewalDate)
}

func TestSubscription_Renew_KeepsMaxMessages(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierPro)
	require.NoError(t, sub.Renew())
	assert.Equal(t, 100, sub.MaxMessages, "MaxMessages must not change on renewal")
}

func TestSubscription_Renew_InactiveSubscription_Succeeds(t *testing.T) {
	// Inactive but not cancelled — renewal job can reactivate it
	sub := newTestSubscription(t, subdomain.TierBasic)
	sub.IsActive = false

	// Should succeed: inactive alone doesn't block renewal (only CancelledAt does)
	err := sub.Renew()
	assert.NoError(t, err)
	assert.True(t, sub.IsActive, "renewal must reactivate the subscription")
}

func TestSubscription_Renew_UpdatesUpdatedAt(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	before := time.Now().UTC().Add(-time.Second)

	require.NoError(t, sub.Renew())
	assert.True(t, sub.UpdatedAt.After(before))
}

// ── Deactivate ────────────────────────────────────────────────────────────────

func TestSubscription_Deactivate_SetsIsActiveFalse(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	assert.True(t, sub.IsActive)

	sub.Deactivate("payment_failure")
	assert.False(t, sub.IsActive)
}

func TestSubscription_Deactivate_UpdatesUpdatedAt(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	before := time.Now().UTC().Add(-time.Second)

	sub.Deactivate("expired")
	assert.True(t, sub.UpdatedAt.After(before))
}

func TestSubscription_Deactivate_DoesNotCancel(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	sub.Deactivate("payment_failure")
	assert.Nil(t, sub.CancelledAt, "Deactivate must not set CancelledAt")
}

// ── HasCapacity boundary cases ────────────────────────────────────────────────

func TestSubscription_HasCapacity_ProAtExactLimit(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierPro)
	sub.MessagesUsed = 100 // exactly at limit
	assert.False(t, sub.HasCapacity(), "at-limit must be false (used < max is the condition)")
}

func TestSubscription_HasCapacity_OneUnderLimit(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierPro)
	sub.MessagesUsed = 99
	assert.True(t, sub.HasCapacity())
}

func TestSubscription_HasCapacity_OverLimit(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	sub.MessagesUsed = 999
	assert.False(t, sub.HasCapacity())
}

func TestSubscription_HasCapacity_ZeroUsed_IsTrue(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	assert.True(t, sub.HasCapacity())
}

// ── RemainingMessages edge cases ──────────────────────────────────────────────

func TestSubscription_RemainingMessages_AtExactZero(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic) // max=10
	sub.MessagesUsed = 10
	assert.Equal(t, 0, sub.RemainingMessages())
}

func TestSubscription_RemainingMessages_OverLimit_ClampsToZero(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic) // max=10
	sub.MessagesUsed = 15                              // shouldn't happen but guard it
	assert.Equal(t, 0, sub.RemainingMessages(), "remaining must not go negative")
}

func TestSubscription_RemainingMessages_Full(t *testing.T) {
	sub := newTestSubscription(t, subdomain.TierBasic)
	sub.MessagesUsed = 0
	assert.Equal(t, 10, sub.RemainingMessages())
}

// ── SubscriptionPolicy – missing coverage ─────────────────────────────────────

func TestSubscriptionPolicy_CanView_SameUser(t *testing.T) {
	p := subdomain.NewSubscriptionPolicy()
	id := uuid.New()
	assert.NoError(t, p.CanView(id, id))
}

func TestSubscriptionPolicy_CanView_DifferentUser(t *testing.T) {
	p := subdomain.NewSubscriptionPolicy()
	assert.Error(t, p.CanView(uuid.New(), uuid.New()))
}

func TestSubscriptionPolicy_CanToggleAutoRenew_DifferentUser(t *testing.T) {
	p := subdomain.NewSubscriptionPolicy()
	assert.Error(t, p.CanToggleAutoRenew(uuid.New(), uuid.New()))
}

func TestSubscriptionPolicy_CanCancel_SameUser_NoError(t *testing.T) {
	p := subdomain.NewSubscriptionPolicy()
	id := uuid.New()
	assert.NoError(t, p.CanCancel(id, id))
}

func TestSubscriptionPolicy_AllMethods_ZeroUUIDs_AllowsWhenEqual(t *testing.T) {
	p := subdomain.NewSubscriptionPolicy()
	zero := uuid.UUID{}
	// Two zero UUIDs are equal — policy should allow it
	assert.NoError(t, p.CanCreate(zero, zero))
	assert.NoError(t, p.CanView(zero, zero))
	assert.NoError(t, p.CanCancel(zero, zero))
	assert.NoError(t, p.CanToggleAutoRenew(zero, zero))
}

package unit

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	chatdomain "github.com/Ammar022/muhammad-ammar-azam/internal/chat/domain"
	subdomain "github.com/Ammar022/muhammad-ammar-azam/internal/subscription/domain"
)

func TestQuotaPolicy_CanSendMessage_SameUser(t *testing.T) {
	policy := chatdomain.NewQuotaPolicy()
	id := uuid.New()
	assert.NoError(t, policy.CanSendMessage(id, id))
}

func TestQuotaPolicy_CanSendMessage_DifferentUser(t *testing.T) {
	policy := chatdomain.NewQuotaPolicy()
	assert.Error(t, policy.CanSendMessage(uuid.New(), uuid.New()))
}

func TestQuotaPolicy_CanViewMessage_SameUser(t *testing.T) {
	policy := chatdomain.NewQuotaPolicy()
	id := uuid.New()
	assert.NoError(t, policy.CanViewMessage(id, id))
}

func TestQuotaPolicy_CanViewMessage_DifferentUser(t *testing.T) {
	policy := chatdomain.NewQuotaPolicy()
	assert.Error(t, policy.CanViewMessage(uuid.New(), uuid.New()))
}

func TestSubscriptionPolicy_CanCreate_SameUser(t *testing.T) {
	policy := subdomain.NewSubscriptionPolicy()
	id := uuid.New()
	assert.NoError(t, policy.CanCreate(id, id))
}

func TestSubscriptionPolicy_CanCreate_DifferentUser(t *testing.T) {
	policy := subdomain.NewSubscriptionPolicy()
	assert.Error(t, policy.CanCreate(uuid.New(), uuid.New()))
}

func TestSubscriptionPolicy_CanCancel_SameUser(t *testing.T) {
	policy := subdomain.NewSubscriptionPolicy()
	id := uuid.New()
	assert.NoError(t, policy.CanCancel(id, id))
}

func TestSubscriptionPolicy_CanCancel_DifferentUser(t *testing.T) {
	policy := subdomain.NewSubscriptionPolicy()
	assert.Error(t, policy.CanCancel(uuid.New(), uuid.New()))
}

func TestSubscriptionPolicy_CanToggleAutoRenew_SameUser(t *testing.T) {
	policy := subdomain.NewSubscriptionPolicy()
	id := uuid.New()
	assert.NoError(t, policy.CanToggleAutoRenew(id, id))
}

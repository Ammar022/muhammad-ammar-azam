// Package dto defines Data Transfer Objects for the subscription module.
package dto

// CreateSubscriptionRequest is the body for POST /api/v1/subscriptions.
type CreateSubscriptionRequest struct {
	// Tier must be one of: "basic", "pro", "enterprise"
	// basic = 10 msgs | pro = 100 msgs | enterprise = unlimited
	Tier string `json:"tier" validate:"required,tier" enums:"basic,pro,enterprise" example:"pro"`
	// BillingCycle must be one of: "monthly", "yearly"
	// Yearly billing applies a 20% discount on the monthly price.
	BillingCycle string `json:"billing_cycle" validate:"required,billing_cycle" enums:"monthly,yearly" example:"monthly"`
	// AutoRenew controls whether the subscription automatically renews at the end of the billing cycle.
	AutoRenew bool `json:"auto_renew" example:"true"`
}

// ToggleAutoRenewRequest is the body for PATCH /api/v1/subscriptions/{id}/auto-renew.
type ToggleAutoRenewRequest struct {
	// Enable sets auto_renew to true or false.
	Enable bool `json:"enable" example:"true"`
}

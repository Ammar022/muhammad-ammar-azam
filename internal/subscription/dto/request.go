package dto

// CreateSubscriptionRequest is the body for POST /api/v1/subscriptions.
type CreateSubscriptionRequest struct {
	Tier         string `json:"tier" validate:"required,tier" enums:"basic,pro,enterprise" example:"pro"`
	BillingCycle string `json:"billing_cycle" validate:"required,billing_cycle" enums:"monthly,yearly" example:"monthly"`
	AutoRenew    bool   `json:"auto_renew" example:"true"`
}

// ToggleAutoRenewRequest is the body for PATCH /api/v1/subscriptions/{id}/auto-renew.
type ToggleAutoRenewRequest struct {
	Enable bool `json:"enable" example:"true"`
}

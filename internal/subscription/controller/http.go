package controller

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/Ammar022/muhammad-ammar-azam/internal/shared/auth"
	apperrors "github.com/Ammar022/muhammad-ammar-azam/internal/shared/errors"
	"github.com/Ammar022/muhammad-ammar-azam/internal/shared/response"
	subdomain "github.com/Ammar022/muhammad-ammar-azam/internal/subscription/domain"
	"github.com/Ammar022/muhammad-ammar-azam/internal/subscription/dto"
)

// SubscriptionController handles all HTTP requests for /api/v1/subscriptions
type SubscriptionController struct {
	service  *subdomain.SubscriptionService
	validate *validator.Validate
}

// NewSubscriptionController creates a SubscriptionController
func NewSubscriptionController(service *subdomain.SubscriptionService) *SubscriptionController {
	v := validator.New()
	if err := v.RegisterValidation("tier", func(fl validator.FieldLevel) bool {
		return subdomain.IsValidTier(fl.Field().String())
	}); err != nil {
		log.Fatalf("failed to register tier validation: %v", err)
	}

	if err := v.RegisterValidation("billing_cycle", func(fl validator.FieldLevel) bool {
		return subdomain.IsValidBillingCycle(fl.Field().String())
	}); err != nil {
		log.Fatalf("failed to register billing_cycle validation: %v", err)
	}
	return &SubscriptionController{
		service:  service,
		validate: v,
	}
}

// Routes registers subscription endpoints on the given chi router
func (c *SubscriptionController) Routes(r chi.Router) {
	r.Post("/", c.createSubscription)
	r.Get("/", c.listSubscriptions)
	r.Get("/{id}", c.getSubscription)
	r.Patch("/{id}/cancel", c.cancelSubscription)
	r.Patch("/{id}/auto-renew", c.toggleAutoRenew)
}

// createSubscription handles POST /api/v1/subscriptions
//
//	@Summary		Create a subscription bundle
//	@Description	Creates a new active subscription for the user. Users may hold multiple active bundles simultaneously. Quota is deducted from the newest bundle that still has capacity.
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string				true	"Unix timestamp in seconds (e.g. 1711234567)"
//	@Param			X-Nonce				header		string				true	"Unique string per request, e.g. a UUID"
//	@Param			body				body		dto.CreateSubscriptionRequest	true	"Subscription details"
//	@Success		201	{object}		response.Envelope{data=subscriptionResponse}	"Subscription created"
//	@Failure		400	{object}		response.Envelope	"Invalid input"
//	@Failure		422	{object}		response.Envelope	"Validation error — see details for field messages"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Security		BearerAuth
//	@Router			/api/v1/subscriptions [post]
func (c *SubscriptionController) createSubscription(w http.ResponseWriter, r *http.Request) {
	claims := auth.MustClaimsFromContext(r.Context())

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req dto.CreateSubscriptionRequest
	if err := dec.Decode(&req); err != nil {
		response.ErrorWithDetails(w, apperrors.ErrInvalidInput, err.Error())
		return
	}
	if err := c.validate.Struct(req); err != nil {
		response.ErrorWithDetails(w, apperrors.ErrValidation, formatValidationErrors(err))
		return
	}

	sub, err := c.service.CreateSubscription(
		r.Context(),
		claims.InternalUserID,
		subdomain.Tier(req.Tier),
		subdomain.BillingCycle(req.BillingCycle),
		req.AutoRenew,
	)
	if err != nil {
		response.Error(w, err)
		return
	}

	response.Created(w, toSubscriptionResponse(sub))
}

// listSubscriptions handles GET /api/v1/subscriptions
//
//	@Summary		List subscriptions
//	@Description	Returns all subscription bundles (active and inactive) for the authenticated user.
//	@Tags			subscriptions
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string	true	"Unix timestamp in seconds (e.g. 1711234567)"
//	@Param			X-Nonce				header		string	true	"Unique string per request, e.g. a UUID"
//	@Success		200	{object}		response.Envelope{data=[]subscriptionResponse}	"List of subscriptions"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Security		BearerAuth
//	@Router			/api/v1/subscriptions [get]
func (c *SubscriptionController) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	claims := auth.MustClaimsFromContext(r.Context())

	subs, err := c.service.ListSubscriptions(r.Context(), claims.InternalUserID)
	if err != nil {
		response.Error(w, err)
		return
	}

	items := make([]subscriptionResponse, 0, len(subs))
	for _, s := range subs {
		items = append(items, toSubscriptionResponse(s))
	}
	response.Success(w, items)
}

// getSubscription handles GET /api/v1/subscriptions/{id}
//
//	@Summary		Get a subscription
//	@Description	Returns a single subscription bundle by ID. Users can only access their own subscriptions.
//	@Tags			subscriptions
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string	true	"Unix timestamp in seconds (e.g. 1711234567)"
//	@Param			X-Nonce				header		string	true	"Unique string per request, e.g. a UUID"
//	@Param			id					path		string	true	"Subscription UUID (e.g. a1b2c3d4-e5f6-7890-abcd-ef1234567890)"
//	@Success		200	{object}		response.Envelope{data=subscriptionResponse}	"Subscription"
//	@Failure		400	{object}		response.Envelope	"Invalid UUID"
//	@Failure		403	{object}		response.Envelope	"Forbidden"
//	@Failure		404	{object}		response.Envelope	"Not found"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Security		BearerAuth
//	@Router			/api/v1/subscriptions/{id} [get]
func (c *SubscriptionController) getSubscription(w http.ResponseWriter, r *http.Request) {
	claims := auth.MustClaimsFromContext(r.Context())

	id, err := parseUUID(r, "id")
	if err != nil {
		response.Error(w, apperrors.ErrInvalidInput)
		return
	}

	sub, err := c.service.GetSubscription(r.Context(), claims.InternalUserID, id)
	if err != nil {
		response.Error(w, err)
		return
	}

	response.Success(w, toSubscriptionResponse(sub))
}

// cancelSubscription handles PATCH /api/v1/subscriptions/{id}/cancel
//
//	@Summary		Cancel a subscription
//	@Description	Marks the subscription as cancelled. Access is preserved until the end of the billing cycle. Auto-renew is disabled. Historical usage data is preserved.
//	@Tags			subscriptions
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string	true	"Unix timestamp in seconds (e.g. 1711234567)"
//	@Param			X-Nonce				header		string	true	"Unique string per request, e.g. a UUID"
//	@Param			id					path		string	true	"Subscription UUID (e.g. a1b2c3d4-e5f6-7890-abcd-ef1234567890)"
//	@Success		200	{object}		response.Envelope{data=subscriptionResponse}	"Cancelled subscription"
//	@Failure		400	{object}		response.Envelope	"Already cancelled or invalid ID"
//	@Failure		403	{object}		response.Envelope	"Forbidden"
//	@Failure		404	{object}		response.Envelope	"Not found"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Security		BearerAuth
//	@Router			/api/v1/subscriptions/{id}/cancel [patch]
func (c *SubscriptionController) cancelSubscription(w http.ResponseWriter, r *http.Request) {
	claims := auth.MustClaimsFromContext(r.Context())

	id, err := parseUUID(r, "id")
	if err != nil {
		response.Error(w, apperrors.ErrInvalidInput)
		return
	}

	sub, err := c.service.CancelSubscription(r.Context(), claims.InternalUserID, id)
	if err != nil {
		response.Error(w, err)
		return
	}

	response.Success(w, toSubscriptionResponse(sub))
}

// toggleAutoRenew handles PATCH /api/v1/subscriptions/{id}/auto-renew
//
//	@Summary		Toggle auto-renew
//	@Description	Enables or disables automatic renewal for a subscription. Cannot be re-enabled on a cancelled subscription.
//	@Tags			subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string					true	"Unix timestamp in seconds (e.g. 1711234567)"
//	@Param			X-Nonce				header		string					true	"Unique string per request, e.g. a UUID"
//	@Param			id					path		string					true	"Subscription UUID (e.g. a1b2c3d4-e5f6-7890-abcd-ef1234567890)"
//	@Param			body				body		dto.ToggleAutoRenewRequest	true	"Set enable to true to turn on auto-renew, false to turn it off"
//	@Success		200	{object}		response.Envelope{data=subscriptionResponse}	"Updated subscription"
//	@Failure		400	{object}		response.Envelope	"Invalid input or subscription is cancelled"
//	@Failure		403	{object}		response.Envelope	"Forbidden"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Security		BearerAuth
//	@Router			/api/v1/subscriptions/{id}/auto-renew [patch]
func (c *SubscriptionController) toggleAutoRenew(w http.ResponseWriter, r *http.Request) {
	claims := auth.MustClaimsFromContext(r.Context())

	id, err := parseUUID(r, "id")
	if err != nil {
		response.Error(w, apperrors.ErrInvalidInput)
		return
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req dto.ToggleAutoRenewRequest
	if err := dec.Decode(&req); err != nil {
		response.ErrorWithDetails(w, apperrors.ErrInvalidInput, err.Error())
		return
	}

	sub, err := c.service.ToggleAutoRenew(r.Context(), claims.InternalUserID, id, req.Enable)
	if err != nil {
		response.Error(w, err)
		return
	}

	response.Success(w, toSubscriptionResponse(sub))
}

type subscriptionResponse struct {
	ID                string  `json:"id" example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890"`
	Tier              string  `json:"tier" example:"pro" enums:"basic,pro,enterprise"`
	BillingCycle      string  `json:"billing_cycle" example:"monthly" enums:"monthly,yearly"`
	AutoRenew         bool    `json:"auto_renew" example:"true"`
	MaxMessages       int     `json:"max_messages" example:"100"`
	MessagesUsed      int     `json:"messages_used" example:"23"`
	RemainingMessages int     `json:"remaining_messages" example:"77"`
	Price             float64 `json:"price" example:"49.99"`
	StartDate         string  `json:"start_date" example:"2024-03-01T00:00:00Z"`
	EndDate           string  `json:"end_date" example:"2024-04-01T00:00:00Z"`
	RenewalDate       string  `json:"renewal_date" example:"2024-04-01T00:00:00Z"`
	IsActive          bool    `json:"is_active" example:"true"`
	CancelledAt       *string `json:"cancelled_at,omitempty" example:"2024-03-15T08:00:00Z"`
	CreatedAt         string  `json:"created_at" example:"2024-03-01T00:00:00Z"`
}

func toSubscriptionResponse(s *subdomain.Subscription) subscriptionResponse {
	r := subscriptionResponse{
		ID:                s.ID.String(),
		Tier:              string(s.Tier),
		BillingCycle:      string(s.BillingCycle),
		AutoRenew:         s.AutoRenew,
		MaxMessages:       s.MaxMessages,
		MessagesUsed:      s.MessagesUsed,
		RemainingMessages: s.RemainingMessages(),
		Price:             s.Price,
		StartDate:         s.StartDate.Format("2006-01-02T15:04:05Z07:00"),
		EndDate:           s.EndDate.Format("2006-01-02T15:04:05Z07:00"),
		RenewalDate:       s.RenewalDate.Format("2006-01-02T15:04:05Z07:00"),
		IsActive:          s.IsActive,
		CreatedAt:         s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if s.CancelledAt != nil {
		t := s.CancelledAt.Format("2006-01-02T15:04:05Z07:00")
		r.CancelledAt = &t
	}
	return r
}

// parseUUID extracts and parses a UUID from the request URL parameter
func parseUUID(r *http.Request, param string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, param))
}

func formatValidationErrors(err error) map[string]string {
	errs := make(map[string]string)
	if ve, ok := err.(validator.ValidationErrors); ok {
		for _, fe := range ve {
			switch fe.Field() {
			case "Tier":
				errs["Tier"] = "tier must be one of: basic, pro, enterprise"
			case "BillingCycle":
				errs["BillingCycle"] = "billing_cycle must be one of: monthly, yearly"
			default:
				errs[fe.Field()] = fe.Tag()
			}
		}
	}
	return errs
}

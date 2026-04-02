package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/auth"
	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/response"
)

// AdminController handles admin-only and observability endpoints.
type AdminController struct {
	db *sqlx.DB
}

// NewAdminController creates an AdminController.
func NewAdminController(db *sqlx.DB) *AdminController {
	return &AdminController{db: db}
}

// Routes registers admin endpoints.  All routes have RequireRole(admin)
// applied at the parent router level.
func (c *AdminController) Routes(r chi.Router) {
	r.Get("/metrics", c.getMetrics)
	r.Get("/users", c.listUsers)
	r.Get("/users/{id}/usage", c.getUserUsage)
}

// metricsResponse is the payload for GET /api/v1/admin/metrics.
type metricsResponse struct {
	TotalUsers              int64            `json:"total_users"`
	TotalMessages           int64            `json:"total_messages"`
	TotalSubscriptions      int64            `json:"total_subscriptions"`
	ActiveSubscriptions     int64            `json:"active_subscriptions"`
	SubscriptionsByTier     map[string]int64 `json:"subscriptions_by_tier"`
	MessagesToday           int64            `json:"messages_today"`
	FreeQuotaUsageThisMonth int64            `json:"free_quota_usage_this_month"`
	GeneratedAt             string           `json:"generated_at"`
}

// getMetrics handles GET /api/v1/admin/metrics
//
//	@Summary		Get system metrics
//	@Description	Returns aggregated usage and subscription metrics. Admin role required.
//	@Tags			admin
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string	true	"Unix timestamp"
//	@Param			X-Nonce				header		string	true	"Unique nonce"
//	@Success		200	{object}		response.Envelope{data=metricsResponse}	"System metrics"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Failure		403	{object}		response.Envelope	"Forbidden — admin role required"
//	@Security		BearerAuth
//	@Router			/api/v1/admin/metrics [get]
func (c *AdminController) getMetrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	metrics := metricsResponse{
		SubscriptionsByTier: make(map[string]int64),
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339),
	}

	// Total users
	_ = c.db.GetContext(ctx, &metrics.TotalUsers, `SELECT COUNT(*) FROM users`)

	// Total and active subscriptions
	_ = c.db.GetContext(ctx, &metrics.TotalSubscriptions, `SELECT COUNT(*) FROM subscriptions`)
	_ = c.db.GetContext(ctx, &metrics.ActiveSubscriptions, `SELECT COUNT(*) FROM subscriptions WHERE is_active = TRUE`)

	// Subscriptions per tier
	rows, err := c.db.QueryContext(ctx, `SELECT tier, COUNT(*) FROM subscriptions GROUP BY tier`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var tier string
			var count int64
			if err := rows.Scan(&tier, &count); err == nil {
				metrics.SubscriptionsByTier[tier] = count
			}
		}
	}

	// Total chat messages
	_ = c.db.GetContext(ctx, &metrics.TotalMessages, `SELECT COUNT(*) FROM chat_messages`)

	// Messages in the last 24 hours
	_ = c.db.GetContext(ctx, &metrics.MessagesToday,
		`SELECT COUNT(*) FROM chat_messages WHERE created_at >= NOW() - INTERVAL '24 hours'`)

	// Free quota usage this calendar month
	currentMonth := time.Now().UTC().Format("2006-01")
	_ = c.db.GetContext(ctx, &metrics.FreeQuotaUsageThisMonth,
		`SELECT COALESCE(SUM(free_messages_used), 0) FROM quota_usages WHERE month = $1`, currentMonth)

	response.Success(w, metrics)
}

// listUsers handles GET /api/v1/admin/users
//
//	@Summary		List all users
//	@Description	Returns all registered users ordered by creation date. Admin role required.
//	@Tags			admin
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string	true	"Unix timestamp"
//	@Param			X-Nonce				header		string	true	"Unique nonce"
//	@Success		200	{object}		response.Envelope	"List of users"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Failure		403	{object}		response.Envelope	"Forbidden — admin role required"
//	@Security		BearerAuth
//	@Router			/api/v1/admin/users [get]
func (c *AdminController) listUsers(w http.ResponseWriter, r *http.Request) {
	type row struct {
		ID         string    `db:"id"`
		ExternalID string    `db:"external_id"`
		Email      string    `db:"email"`
		Role       string    `db:"role"`
		CreatedAt  time.Time `db:"created_at"`
	}

	var users []row
	if err := c.db.SelectContext(r.Context(), &users,
		`SELECT id, external_id, email, role, created_at FROM users ORDER BY created_at DESC`); err != nil {
		response.Error(w, apperrors.ErrInternal)
		return
	}

	type userResp struct {
		ID         string `json:"id"`
		ExternalID string `json:"external_id"`
		Email      string `json:"email"`
		Role       string `json:"role"`
		CreatedAt  string `json:"created_at"`
	}
	items := make([]userResp, 0, len(users))
	for _, u := range users {
		items = append(items, userResp{
			ID:         u.ID,
			ExternalID: u.ExternalID,
			Email:      u.Email,
			Role:       u.Role,
			CreatedAt:  u.CreatedAt.Format(time.RFC3339),
		})
	}
	response.Success(w, items)
}

// userUsageResponse holds usage detail for a single user.
type userUsageResponse struct {
	UserID           string            `json:"user_id"`
	FreeMessagesUsed int               `json:"free_messages_used"`
	TotalMessages    int64             `json:"total_messages"`
	Subscriptions    []subUsageSummary `json:"subscriptions"`
}

type subUsageSummary struct {
	ID           string `json:"id"`
	Tier         string `json:"tier"`
	MessagesUsed int    `json:"messages_used"`
	MaxMessages  int    `json:"max_messages"`
	IsActive     bool   `json:"is_active"`
}

// subRow is a DB row for subscription data.
type subRow struct {
	ID           string `db:"id"`
	Tier         string `db:"tier"`
	MessagesUsed int    `db:"messages_used"`
	MaxMessages  int    `db:"max_messages"`
	IsActive     bool   `db:"is_active"`
}

// newSubUsageSummary converts a subRow to subUsageSummary.
func newSubUsageSummary(s subRow) subUsageSummary {
	return subUsageSummary(s)
}

// getUserUsage handles GET /api/v1/admin/users/{id}/usage
//
//	@Summary		Get user usage
//	@Description	Returns detailed usage stats for a specific user: free quota used this month, total messages, and all subscription bundles. Admin role required.
//	@Tags			admin
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string	true	"Unix timestamp"
//	@Param			X-Nonce				header		string	true	"Unique nonce"
//	@Param			id					path		string	true	"User UUID"
//	@Success		200	{object}		response.Envelope{data=userUsageResponse}	"User usage details"
//	@Failure		400	{object}		response.Envelope	"Invalid user ID"
//	@Failure		401	{object}		response.Envelope	"Unauthorized"
//	@Failure		403	{object}		response.Envelope	"Forbidden — admin role required"
//	@Security		BearerAuth
//	@Router			/api/v1/admin/users/{id}/usage [get]
func (c *AdminController) getUserUsage(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		response.Error(w, apperrors.ErrInvalidInput)
		return
	}

	usage := userUsageResponse{UserID: userID}

	// Free quota this month
	currentMonth := time.Now().UTC().Format("2006-01")
	_ = c.db.QueryRowContext(r.Context(),
		`SELECT COALESCE(free_messages_used, 0) FROM quota_usages WHERE user_id = $1 AND month = $2`,
		userID, currentMonth,
	).Scan(&usage.FreeMessagesUsed)

	// Total messages
	_ = c.db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM chat_messages WHERE user_id = $1`, userID,
	).Scan(&usage.TotalMessages)

	// Subscriptions
	var subs []subRow
	_ = c.db.SelectContext(r.Context(), &subs,
		`SELECT id, tier, messages_used, max_messages, is_active
		 FROM subscriptions WHERE user_id = $1 ORDER BY created_at DESC`, userID)

	for _, s := range subs {
		usage.Subscriptions = append(usage.Subscriptions, newSubUsageSummary(s))
	}

	response.Success(w, usage)
}

// HealthController handles the /health endpoint.
// This is a public (unauthenticated) endpoint used by load balancers and
// monitoring systems.
type HealthController struct {
	db      *sqlx.DB
	version string
}

// NewHealthController creates a HealthController.
func NewHealthController(db *sqlx.DB, version string) *HealthController {
	return &HealthController{db: db, version: version}
}

// healthResponse is the payload for GET /health.
type healthResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Checks  map[string]string `json:"checks"`
	Uptime  string            `json:"uptime"`
}

var startTime = time.Now()

// Health handles GET /health.
//
//	@Summary		Health check
//	@Description	Returns 200 OK with subsystem status when healthy, 503 when degraded. Used by load balancers and monitoring systems. No authentication required.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}		map[string]interface{}	"All systems healthy"
//	@Failure		503	{object}		map[string]interface{}	"One or more subsystems degraded"
//	@Router			/health [get]
func (c *HealthController) Health(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{
		"database": "ok",
	}
	overallStatus := "ok"

	// Check DB connectivity
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := c.db.PingContext(ctx); err != nil {
		checks["database"] = "error: " + err.Error()
		overallStatus = "degraded"
	}

	statusCode := http.StatusOK
	if overallStatus != "ok" {
		statusCode = http.StatusServiceUnavailable
	}

	payload := healthResponse{
		Status:  overallStatus,
		Version: c.version,
		Checks:  checks,
		Uptime:  time.Since(startTime).Round(time.Second).String(),
	}

	// Don't use response.Success here — we need to set a custom status code
	response.JSON(w, statusCode, map[string]interface{}{
		"success": overallStatus == "ok",
		"data":    payload,
	})
}

// RequireAuth is a convenience wrapper so the admin router can enforce
// both JWT validation and admin role in one call.
func RequireAdminRole(next http.Handler) http.Handler {
	return auth.RequireRole(auth.RoleAdmin)(next)
}

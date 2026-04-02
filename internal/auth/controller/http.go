// Package controller implements admin-only HTTP handlers for user management.
package controller

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/auth"
	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/response"
	userdomain "github.com/Ammar022/secure-ai-chat-backend/internal/user/domain"
	userrepo "github.com/Ammar022/secure-ai-chat-backend/internal/user/repository"
)

// AdminController exposes an admin-only endpoint to manage user roles.
type AdminController struct {
	userRepo userrepo.UserRepository
}

// NewGoogleAdminController creates the AdminController.
// The name is kept for backward-compat with existing main.go wiring.
func NewGoogleAdminController(userRepo userrepo.UserRepository) *AdminController {
	return &AdminController{userRepo: userRepo}
}

// Routes registers the admin-scoped auth management endpoints.
func (c *AdminController) Routes(r chi.Router) {
	r.Patch("/users/{id}/role", c.SetRole)
}

// setRoleRequest is the body for PATCH /api/v1/admin/users/{id}/role.
type setRoleRequest struct {
	// Role must be "user" or "admin".
	Role string `json:"role" enums:"user,admin" example:"admin"`
}

// SetRole changes the role of any user.
//
//	@Summary		Set user role
//	@Description	Promotes or demotes a user to/from admin. Role must be "user" or "admin". Admin role required.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			X-Request-Timestamp	header		string				true	"Unix timestamp (seconds since epoch, e.g. 1711234567)"
//	@Param			X-Nonce				header		string				true	"Unique string per request (e.g. uuid or random hex)"
//	@Param			id					path		string				true	"Target user UUID"
//	@Param			body				body		setRoleRequest			true	"Role assignment payload"
//	@Success		200	{object}		response.Envelope				"Updated user"
//	@Failure		400	{object}		response.Envelope				"Invalid role or user ID"
//	@Failure		401	{object}		response.Envelope				"Unauthorized"
//	@Failure		403	{object}		response.Envelope				"Forbidden — admin role required"
//	@Failure		404	{object}		response.Envelope				"User not found"
//	@Security		BearerAuth
//	@Router			/api/v1/admin/users/{id}/role [patch]
func (c *AdminController) SetRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		response.Error(w, apperrors.ErrUnauthorized)
		return
	}

	if !claims.HasRole(auth.RoleAdmin) {
		response.Error(w, apperrors.ErrForbidden)
		return
	}

	targetID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		response.Error(w, apperrors.ErrInvalidInput)
		return
	}

	var body setRoleRequest
	if err := response.DecodeJSON(r, &body); err != nil {
		response.Error(w, apperrors.ErrInvalidInput)
		return
	}

	role := userdomain.Role(body.Role)
	if role != userdomain.RoleUser && role != userdomain.RoleAdmin {
		response.Error(w, apperrors.New(http.StatusBadRequest, "INVALID_ROLE", "role must be 'user' or 'admin'"))
		return
	}

	user, err := c.userRepo.FindByID(r.Context(), targetID)
	if err != nil {
		response.Error(w, apperrors.ErrNotFound)
		return
	}

	user.Role = role
	user.UpdatedAt = time.Now().UTC()
	if _, err := c.userRepo.Upsert(r.Context(), user); err != nil {
		log.Error().Err(err).Msg("SetRole: failed to update user")
		response.Error(w, apperrors.ErrInternal)
		return
	}

	response.JSON(w, http.StatusOK, map[string]interface{}{
		"id":   user.ID.String(),
		"role": string(user.Role),
	})
}

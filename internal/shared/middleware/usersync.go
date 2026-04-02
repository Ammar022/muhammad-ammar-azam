package middleware

import (
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/auth"
	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/response"
	userdomain "github.com/Ammar022/secure-ai-chat-backend/internal/user/domain"
	userrepo "github.com/Ammar022/secure-ai-chat-backend/internal/user/repository"
)

// UserSync middleware runs immediately after JWT validation on every
// authenticated request.  It implements the "upsert-on-login" pattern:
//
//  1. Extracts the Auth0 subject + email from the validated Claims.
//  2. Upserts the user in the local users table (insert on first login,
//     update email on subsequent logins).
//  3. Populates Claims.InternalUserID with the internal UUID so every
//     downstream handler can access it without an extra DB call.
//  4. Promotes the local DB role to admin if the user has that role stored
//     locally, allowing admin promotion without reissuing Auth0 JWTs.
//
// Dependency direction (no cycles):
//
//	shared/middleware → shared/auth (for Claims)
//	shared/middleware → user/domain  (for User entity)
//	shared/middleware → user/repository (for Upsert)
func UserSync(repo userrepo.UserRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := auth.ClaimsFromContext(r.Context())
			if !ok {
				response.Error(w, apperrors.ErrUnauthorized)
				return
			}

			// Determine the role to seed into the DB on first creation.
			// Auth0 roles (from JWT) take precedence for initial seeding.
			seedRole := userdomain.RoleUser
			for _, role := range claims.Roles {
				if role == auth.RoleAdmin {
					seedRole = userdomain.RoleAdmin
					break
				}
			}

			user, err := repo.Upsert(r.Context(), &userdomain.User{
				ExternalID: claims.Subject,
				Email:      claims.Email,
				Role:       seedRole,
			})
			if err != nil {
				log.Ctx(r.Context()).Error().Err(err).
					Str("external_id", claims.Subject).
					Msg("usersync: upsert failed")
				response.Error(w, apperrors.ErrInternal)
				return
			}

			// Inject the local UUID into claims so handlers use it directly
			claims.InternalUserID = user.ID

			// If the local DB marks this user as admin but the JWT didn't include
			// the admin role, promote them now.  This allows admins to be granted
			// access by updating the DB without waiting for token expiry.
			if user.Role == userdomain.RoleAdmin {
				hasAdmin := false
				for _, role := range claims.Roles {
					if role == auth.RoleAdmin {
						hasAdmin = true
						break
					}
				}
				if !hasAdmin {
					claims.Roles = append(claims.Roles, auth.RoleAdmin)
				}
			}

			ctx := auth.WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

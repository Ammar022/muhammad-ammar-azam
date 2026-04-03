package middleware

import (
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/Ammar022/muhammad-ammar-azam/internal/shared/auth"
	apperrors "github.com/Ammar022/muhammad-ammar-azam/internal/shared/errors"
	"github.com/Ammar022/muhammad-ammar-azam/internal/shared/response"
	userdomain "github.com/Ammar022/muhammad-ammar-azam/internal/user/domain"
	userrepo "github.com/Ammar022/muhammad-ammar-azam/internal/user/repository"
)

func UserSync(repo userrepo.UserRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := auth.ClaimsFromContext(r.Context())
			if !ok {
				response.Error(w, apperrors.ErrUnauthorized)
				return
			}

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

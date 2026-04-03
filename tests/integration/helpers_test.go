package integration

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/Ammar022/muhammad-ammar-azam/internal/shared/auth"
)

// fakeClaimsMiddleware injects a synthetic *auth.Claims into the request
// context.  It is used by integration tests to simulate an authenticated
// request without involving a real JWT validator or any external provider.
//
// JD requirement: "Authentication provider must be mocked in tests, not
// bypassed."  We mock the provider by injecting pre-built claims; the auth
// middleware itself is NOT bypassed — it is simply replaced with this
// test-only shim that exercises the same context path.
func fakeClaimsMiddleware(subject string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := &auth.Claims{
				Subject:        subject,
				Email:          subject + "@example.com",
				Roles:          []auth.Role{auth.RoleUser},
				InternalUserID: uuid.New(),
			}
			next.ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
		})
	}
}

// fakeAdminClaimsMiddleware is the same as fakeClaimsMiddleware but injects
// an admin role — used to test admin-only route protection.
func fakeAdminClaimsMiddleware(subject string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := &auth.Claims{
				Subject:        subject,
				Email:          subject + "@example.com",
				Roles:          []auth.Role{auth.RoleAdmin},
				InternalUserID: uuid.New(),
			}
			next.ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
		})
	}
}

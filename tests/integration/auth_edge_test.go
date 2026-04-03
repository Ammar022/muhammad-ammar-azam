package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ammar022/muhammad-ammar-azam/internal/shared/auth"
	"github.com/Ammar022/muhammad-ammar-azam/internal/shared/config"
)

// ── Wrong audience ────────────────────────────────────────────────────────────

func TestAuthMiddleware_RejectsTokenWithWrongAudience(t *testing.T) {
	r, makeToken := buildAuthRouter(t, okHandler)

	// Build a token manually with the wrong audience
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwksSrv := mockJWKSServer(t, privateKey)
	cfg := config.Auth0Config{
		Domain:     jwksSrv.URL,
		Audience:   testAudience,
		RolesClaim: testRolesClaim,
	}
	issuer := cfg.Issuer()

	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience([]string{"https://wrong-audience"}).
		Subject("sub-wrong-aud").
		Claim("email", "user@example.com").
		Claim(testRolesClaim, []interface{}{"user"}).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	require.NoError(t, err)

	privJWK, err := jwk.FromRaw(privateKey)
	require.NoError(t, err)
	require.NoError(t, privJWK.Set(jwk.KeyIDKey, "test-key-id"))

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK))
	require.NoError(t, err)

	// This token is signed but targets the wrong audience
	_ = makeToken // the router was built with testAudience
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+string(signed))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ── Token without roles claim → defaults to "user" role ──────────────────────

func TestAuthMiddleware_TokenWithoutRolesClaim_DefaultsToUserRole(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwksSrv := mockJWKSServer(t, privateKey)
	cfg := config.Auth0Config{
		Domain:     jwksSrv.URL,
		Audience:   testAudience,
		RolesClaim: testRolesClaim,
	}
	issuer := cfg.Issuer()

	validator, err := auth.NewValidator(context.Background(), cfg)
	require.NoError(t, err)

	var capturedRoles []auth.Role
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if claims, ok := auth.ClaimsFromContext(r.Context()); ok {
			capturedRoles = claims.Roles
		}
		w.WriteHeader(http.StatusOK)
	})

	router := chi.NewRouter()
	router.Use(validator.Middleware)
	router.Get("/protected", handler)

	// Build token WITHOUT the roles claim
	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience([]string{testAudience}).
		Subject("no-roles-user").
		Claim("email", "noroles@example.com").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	require.NoError(t, err)

	privJWK, err := jwk.FromRaw(privateKey)
	require.NoError(t, err)
	require.NoError(t, privJWK.Set(jwk.KeyIDKey, "test-key-id"))

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+string(signed))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, capturedRoles, 1, "should default to one role")
	assert.Equal(t, auth.RoleUser, capturedRoles[0], "missing roles claim must default to 'user'")
}

// ── Token without email claim ────────────────────────────────────────────────

func TestAuthMiddleware_TokenWithoutEmailClaim_EmailIsEmpty(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwksSrv := mockJWKSServer(t, privateKey)
	cfg := config.Auth0Config{
		Domain:     jwksSrv.URL,
		Audience:   testAudience,
		RolesClaim: testRolesClaim,
	}
	issuer := cfg.Issuer()

	validator, err := auth.NewValidator(context.Background(), cfg)
	require.NoError(t, err)

	var capturedEmail string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if claims, ok := auth.ClaimsFromContext(r.Context()); ok {
			capturedEmail = claims.Email
		}
		w.WriteHeader(http.StatusOK)
	})

	router := chi.NewRouter()
	router.Use(validator.Middleware)
	router.Get("/protected", handler)

	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience([]string{testAudience}).
		Subject("no-email-user").
		// No "email" claim
		Claim(testRolesClaim, []interface{}{"user"}).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	require.NoError(t, err)

	privJWK, err := jwk.FromRaw(privateKey)
	require.NoError(t, err)
	require.NoError(t, privJWK.Set(jwk.KeyIDKey, "test-key-id"))

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+string(signed))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "", capturedEmail, "missing email must result in empty string, not panic")
}

// ── Token with multiple roles ─────────────────────────────────────────────────

func TestAuthMiddleware_TokenWithMultipleRoles_AllCaptured(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwksSrv := mockJWKSServer(t, privateKey)
	cfg := config.Auth0Config{
		Domain:     jwksSrv.URL,
		Audience:   testAudience,
		RolesClaim: testRolesClaim,
	}
	issuer := cfg.Issuer()

	validator, err := auth.NewValidator(context.Background(), cfg)
	require.NoError(t, err)

	var capturedClaims *auth.Claims
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedClaims, _ = auth.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	router := chi.NewRouter()
	router.Use(validator.Middleware)
	router.Get("/protected", handler)

	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience([]string{testAudience}).
		Subject("multi-role-user").
		Claim("email", "admin@example.com").
		Claim(testRolesClaim, []interface{}{"user", "admin"}).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(time.Hour)).
		Build()
	require.NoError(t, err)

	privJWK, err := jwk.FromRaw(privateKey)
	require.NoError(t, err)
	require.NoError(t, privJWK.Set(jwk.KeyIDKey, "test-key-id"))

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+string(signed))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, capturedClaims)
	assert.True(t, capturedClaims.IsAdmin(), "admin role should be captured")
	assert.True(t, capturedClaims.HasRole(auth.RoleUser), "user role should also be captured")
}

// ── Garbage token ─────────────────────────────────────────────────────────────

func TestAuthMiddleware_RejectsGarbageToken(t *testing.T) {
	r, _ := buildAuthRouter(t, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer thisisnotajwt")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_RejectsTokenWithOnlyTwoParts(t *testing.T) {
	r, _ := buildAuthRouter(t, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer header.payload")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuthMiddleware_RejectsBearerWithOnlySpaces(t *testing.T) {
	r, _ := buildAuthRouter(t, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer    ")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ── RBAC edge cases ───────────────────────────────────────────────────────────

func TestRBACMiddleware_AllowsUserOnUserRoute(t *testing.T) {
	r := chi.NewRouter()
	r.Use(fakeClaimsMiddleware("user-subject"))
	r.Use(auth.RequireRole(auth.RoleUser))
	r.Get("/user-area", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/user-area", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRBACMiddleware_AllowsAdminOnUserRoute(t *testing.T) {
	r := chi.NewRouter()
	r.Use(fakeAdminClaimsMiddleware("admin-subject"))
	r.Use(auth.RequireRole(auth.RoleUser)) // admin has all roles effectively? No — RBAC checks exact match
	r.Get("/user-area", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/user-area", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// Admin doesn't have RoleUser in their claims (fakeAdminClaimsMiddleware only sets admin)
	// So RequireRole(user) should block admin unless admin also has user role
	// This tests the REAL behavior - admin-only claims DO NOT implicitly have user role
	assert.Equal(t, http.StatusForbidden, rec.Code, "admin-only token must not pass user-role check unless user role is also present")
}

func TestRBACMiddleware_NoClaimsInContext_Returns401(t *testing.T) {
	r := chi.NewRouter()
	// No claims middleware — context has no claims
	r.Use(auth.RequireRole(auth.RoleAdmin))
	r.Get("/admin", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRBACMiddleware_RequireAdminOrUser_AdminPasses(t *testing.T) {
	r := chi.NewRouter()
	r.Use(fakeAdminClaimsMiddleware("admin-sub"))
	r.Use(auth.RequireRole(auth.RoleAdmin, auth.RoleUser))
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRBACMiddleware_RequireAdminOrUser_UserPasses(t *testing.T) {
	r := chi.NewRouter()
	r.Use(fakeClaimsMiddleware("user-sub"))
	r.Use(auth.RequireRole(auth.RoleAdmin, auth.RoleUser))
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ── Claims fully populated ────────────────────────────────────────────────────

func TestAuthMiddleware_AllClaimsPopulated_SubjectEmailRoles(t *testing.T) {
	var capturedClaims *auth.Claims
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedClaims, _ = auth.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	r, makeToken := buildAuthRouter(t, handler)
	token := makeToken("auth0|user-complete", "complete@example.com", "user", false)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, capturedClaims)
	assert.Equal(t, "auth0|user-complete", capturedClaims.Subject)
	assert.Equal(t, "complete@example.com", capturedClaims.Email)
	assert.NotEmpty(t, capturedClaims.Roles)
}

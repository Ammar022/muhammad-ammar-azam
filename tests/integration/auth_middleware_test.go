package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
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

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/auth"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/config"
)

// ── Mocked Auth0 provider tests ───────────────────────────────────────────────
//
// JD requirement: "Authentication provider must be mocked in tests, not
// bypassed."
//
// These tests exercise the full JWT middleware stack using RS256 tokens signed
// with a locally-generated RSA key pair.  A mock httptest.Server serves the
// JWKS endpoint so the real Auth0 provider is never contacted — it is fully
// mocked, satisfying the JD requirement.

const testRolesClaim = "https://api.test.example.com/roles"
const testAudience = "https://chat-api-test"

// mockJWKSServer starts an httptest.Server that serves a JWKS built from the
// given RSA public key on any path.
func mockJWKSServer(t *testing.T, privateKey *rsa.PrivateKey) *httptest.Server {
	t.Helper()

	rawKey, err := jwk.FromRaw(privateKey.Public())
	require.NoError(t, err)
	require.NoError(t, rawKey.Set(jwk.KeyIDKey, "test-key-id"))
	require.NoError(t, rawKey.Set(jwk.AlgorithmKey, jwa.RS256))

	keySet := jwk.NewSet()
	require.NoError(t, keySet.AddKey(rawKey))

	keySetBytes, err := json.Marshal(keySet)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(keySetBytes)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// makeRS256Token signs a JWT with the given RSA private key.
// The kid header in the token matches the kid registered in the mock JWKS.
func makeRS256Token(t *testing.T, privateKey *rsa.PrivateKey, issuer, subject, email, role string, expired bool) string {
	t.Helper()

	exp := time.Now().Add(24 * time.Hour)
	if expired {
		exp = time.Now().Add(-1 * time.Hour)
	}

	token, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience([]string{testAudience}).
		Subject(subject).
		Claim("email", email).
		Claim(testRolesClaim, []interface{}{role}).
		IssuedAt(time.Now()).
		Expiration(exp).
		Build()
	require.NoError(t, err)

	// Wrap the private key as a JWK and set the kid so the token header
	// matches the entry in the mock JWKS (required for key lookup to succeed).
	privJWK, err := jwk.FromRaw(privateKey)
	require.NoError(t, err)
	require.NoError(t, privJWK.Set(jwk.KeyIDKey, "test-key-id"))

	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, privJWK))
	require.NoError(t, err)
	return string(signed)
}

// buildAuthRouter starts a mock JWKS server and wires a real auth.Validator
// against it, then returns the router and helpers needed to sign test tokens.
func buildAuthRouter(t *testing.T, handler http.Handler) (http.Handler, func(subject, email, role string, expired bool) string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	jwksSrv := mockJWKSServer(t, privateKey)

	// Domain is set to the test server base URL — JWKSEndpoint() supports
	// full http:// URLs so the validator fetches from the mock instead of Auth0.
	cfg := config.Auth0Config{
		Domain:     jwksSrv.URL,
		Audience:   testAudience,
		RolesClaim: testRolesClaim,
	}
	issuer := cfg.Issuer() // == jwksSrv.URL + "/"

	validator, err := auth.NewValidator(context.Background(), cfg)
	require.NoError(t, err)

	r := chi.NewRouter()
	r.Use(validator.Middleware)
	r.Handle("/protected", handler)

	makeToken := func(subject, email, role string, expired bool) string {
		return makeRS256Token(t, privateKey, issuer, subject, email, role, expired)
	}
	return r, makeToken
}

// TestAuthMiddleware_RejectsRequestWithNoToken verifies that an unauthenticated
// request receives 401.
func TestAuthMiddleware_RejectsRequestWithNoToken(t *testing.T) {
	r, _ := buildAuthRouter(t, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestAuthMiddleware_RejectsExpiredToken verifies that an expired JWT is
// rejected with 401 even though the signature is valid.
func TestAuthMiddleware_RejectsExpiredToken(t *testing.T) {
	r, makeToken := buildAuthRouter(t, okHandler)

	token := makeToken("sub-expired", "expired@example.com", "user", true)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestAuthMiddleware_RejectsTokenSignedWithDifferentKey verifies that a token
// signed by an unknown RSA key is rejected.
func TestAuthMiddleware_RejectsTokenSignedWithDifferentKey(t *testing.T) {
	r, _ := buildAuthRouter(t, okHandler)

	// Sign with a completely different key — validator's JWKS won't match.
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	token, err := jwt.NewBuilder().
		Issuer("https://attacker.example/").
		Subject("sub-tampered").
		Expiration(time.Now().Add(time.Hour)).
		Build()
	require.NoError(t, err)
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256, otherKey))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+string(signed))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestAuthMiddleware_AcceptsValidToken verifies that a well-formed,
// non-expired, correctly-signed RS256 token is accepted and claims are
// injected into the request context.
func TestAuthMiddleware_AcceptsValidToken(t *testing.T) {
	var capturedSubject string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.ClaimsFromContext(r.Context())
		if ok {
			capturedSubject = claims.Subject
		}
		w.WriteHeader(http.StatusOK)
	})

	r, makeToken := buildAuthRouter(t, handler)

	token := makeToken("auth0|12345", "user@example.com", "user", false)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "auth0|12345", capturedSubject, "subject must be injected into context")
}

// TestAuthMiddleware_RejectsMalformedBearerHeader verifies that a malformed
// Authorization header (not "Bearer <token>") returns 401.
func TestAuthMiddleware_RejectsMalformedBearerHeader(t *testing.T) {
	r, _ := buildAuthRouter(t, okHandler)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // Basic auth — wrong scheme
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestRBACMiddleware_BlocksUserFromAdminRoute verifies that RequireRole(admin)
// returns 403 for a token with only the "user" role.
func TestRBACMiddleware_BlocksUserFromAdminRoute(t *testing.T) {
	r := chi.NewRouter()
	r.Use(fakeClaimsMiddleware("user-subject")) // inject user-role claims
	r.Use(auth.RequireRole(auth.RoleAdmin))     // require admin
	r.Get("/admin", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRBACMiddleware_AllowsAdminToAdminRoute verifies that a token carrying
// the "admin" role passes the RequireRole(admin) check.
func TestRBACMiddleware_AllowsAdminToAdminRoute(t *testing.T) {
	r := chi.NewRouter()
	r.Use(fakeAdminClaimsMiddleware("admin-subject")) // inject admin-role claims
	r.Use(auth.RequireRole(auth.RoleAdmin))
	r.Get("/admin", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

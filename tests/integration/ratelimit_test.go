package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/middleware"
)

// TestRateLimitByIP_BlocksAfterLimit verifies that once a client exceeds the
// configured per-minute request limit, subsequent requests receive 429.
func TestRateLimitByIP_BlocksAfterLimit(t *testing.T) {
	const rpm = 3

	r := chi.NewRouter()
	r.Use(middleware.RateLimitByIP(rpm))
	r.Get("/", okHandler)

	// First `rpm` requests must succeed (burst = rpm)
	for i := 0; i < rpm; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d should be allowed", i+1)
	}

	// The (rpm+1)th request must be rate-limited
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "request after limit should be blocked")
	assert.Equal(t, "60", rec.Header().Get("Retry-After"), "Retry-After header must be set")
}

// TestRateLimitByIP_DifferentIPsHaveIndependentBuckets verifies that rate
// limiting is keyed per IP — one client exhausting its quota must not affect
// another client's quota.
func TestRateLimitByIP_DifferentIPsHaveIndependentBuckets(t *testing.T) {
	const rpm = 2

	r := chi.NewRouter()
	r.Use(middleware.RateLimitByIP(rpm))
	r.Get("/", okHandler)

	// Exhaust quota for client A
	for i := 0; i < rpm+1; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.10:1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	// Client B must still be allowed
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.20:5678"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code, "a different IP should not be rate-limited")
}

// TestRateLimitByUser_UsesUserIDAsKey verifies that per-user limiting keys off
// the user claim injected into context, not the IP address.
func TestRateLimitByUser_UsesUserIDAsKey(t *testing.T) {
	const rpm = 2

	r := chi.NewRouter()
	// Inject fake claims so RateLimitByUser can read the subject
	r.Use(fakeClaimsMiddleware("user-alpha"))
	r.Use(middleware.RateLimitByUser(rpm))
	r.Get("/", okHandler)

	// Exhaust user-alpha's quota
	for i := 0; i < rpm; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d should succeed", i+1)
	}

	// One more must be blocked for user-alpha
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

// TestRateLimitByUser_DifferentUsersHaveIndependentBuckets verifies isolation
// between two separate authenticated users.
func TestRateLimitByUser_DifferentUsersHaveIndependentBuckets(t *testing.T) {
	const rpm = 1

	// Build two routers — one per user — so each has an isolated limiter store
	makeRouter := func(subject string) http.Handler {
		r := chi.NewRouter()
		r.Use(fakeClaimsMiddleware(subject))
		r.Use(middleware.RateLimitByUser(rpm))
		r.Get("/", okHandler)
		return r
	}

	routerA := makeRouter("user-a")
	routerB := makeRouter("user-b")

	// Exhaust user-a
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1111"
	rec := httptest.NewRecorder()
	routerA.ServeHTTP(rec, req)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.1:1111"
	rec2 := httptest.NewRecorder()
	routerA.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code, "user-a should be rate-limited")

	// user-b on the same IP must still be allowed
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "10.0.0.1:1111"
	rec3 := httptest.NewRecorder()
	routerB.ServeHTTP(rec3, req3)
	assert.Equal(t, http.StatusOK, rec3.Code, "user-b should not be affected by user-a's limit")
}

// TestRateLimitAuthEndpoint_StricterLimit verifies that auth endpoints have
// their own (stricter) limiter independent of the global IP limiter.
func TestRateLimitAuthEndpoint_StricterLimit(t *testing.T) {
	const authRPM = 2

	r := chi.NewRouter()
	r.Route("/auth", func(r chi.Router) {
		r.Use(middleware.RateLimitByIP(authRPM))
		r.Get("/google/login", okHandler)
	})

	for i := 0; i < authRPM; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
		req.RemoteAddr = fmt.Sprintf("192.168.1.1:%d", 2000+i)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Exceeds auth limit
	req := httptest.NewRequest(http.MethodGet, "/auth/google/login", nil)
	req.RemoteAddr = "192.168.1.1:9999"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

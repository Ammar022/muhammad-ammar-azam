package integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/middleware"
)

// ── RequireJSON edge cases ────────────────────────────────────────────────────

func TestRequireJSON_AcceptsContentTypeWithCharset(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Post("/", okHandler)

	body := strings.NewReader(`{"key":"value"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "charset suffix must not break content-type check")
}

func TestRequireJSON_RejectsTextPlain_OnPost(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("plain"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestRequireJSON_RejectsFormEncoded_OnPost(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("a=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestRequireJSON_RejectsMultipart_OnPost(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=xxx")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestRequireJSON_RejectsNoContentType_OnPut(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Put("/", okHandler)

	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestRequireJSON_RejectsNoContentType_OnPatch(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Patch("/", okHandler)

	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestRequireJSON_AllowsDelete_WithoutContentType(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Delete("/", okHandler)

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "DELETE has no body — Content-Type must not be required")
}

func TestRequireJSON_AllowsOptions_WithoutContentType(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "OPTIONS preflight must not require Content-Type")
}

// ── SecureHeaders coverage ────────────────────────────────────────────────────

func TestSecureHeaders_PresentOnPost(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.SecureHeaders)
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
}

func TestSecureHeaders_ReferrerPolicy_Present(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.SecureHeaders)
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
}

func TestSecureHeaders_PermissionsPolicy_Present(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.SecureHeaders)
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	pp := rec.Header().Get("Permissions-Policy")
	assert.Contains(t, pp, "geolocation=()")
	assert.Contains(t, pp, "microphone=()")
	assert.Contains(t, pp, "camera=()")
}

func TestSecureHeaders_ServerHeader_IsBlank(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.SecureHeaders)
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Empty(t, rec.Header().Get("Server"), "Server header must be empty to prevent version disclosure")
}

func TestSecureHeaders_CacheControl_NoStore(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.SecureHeaders)
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func TestSecureHeaders_CSP_DeniesFraming(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.SecureHeaders)
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	assert.Contains(t, csp, "frame-ancestors 'none'")
}

func TestSecureHeaders_HSTS_IncludesSubDomains(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.SecureHeaders)
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	hsts := rec.Header().Get("Strict-Transport-Security")
	assert.Contains(t, hsts, "includeSubDomains")
	assert.Contains(t, hsts, "max-age=31536000")
}

// ── RequestSizeLimit boundary tests ──────────────────────────────────────────

func TestRequestSizeLimit_ExactlyAtLimit_ContentLength_Rejected(t *testing.T) {
	const maxBytes int64 = 100

	r := chi.NewRouter()
	r.Use(middleware.RequestSizeLimit(maxBytes))
	r.Post("/", okHandler)

	body := strings.NewReader(strings.Repeat("x", int(maxBytes)+1))
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.ContentLength = maxBytes + 1 // declare it explicitly
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestRequestSizeLimit_OneLessThanLimit_Allowed(t *testing.T) {
	const maxBytes int64 = 100

	r := chi.NewRouter()
	r.Use(middleware.RequestSizeLimit(maxBytes))
	r.Post("/", okHandler)

	body := strings.NewReader(strings.Repeat("x", int(maxBytes)-1))
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequestSizeLimit_EmptyBody_Allowed(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequestSizeLimit(100))
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ── Timeout edge cases ────────────────────────────────────────────────────────

func TestTimeout_HandlerCompletesBeforeDeadline_Returns200(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.Timeout(500 * time.Millisecond))
	r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fast handler — completes well before timeout
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestTimeout_HandlerWritesBeforeTimeout_BodyPreserved(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.Timeout(500 * time.Millisecond))
	r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`hello`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "hello", rec.Body.String())
}

func TestTimeout_VeryShortTimeout_SlowHandler_Returns504(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.Timeout(10 * time.Millisecond))
	r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
}

// ── RequestID edge cases ──────────────────────────────────────────────────────

func TestRequestID_EmptyHeaderValue_GeneratesNewID(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "") // present but empty
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// Should generate a new UUID since the value was empty
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
}

func TestRequestID_UniqueIDPerRequest(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/", okHandler)

	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		id := rec.Header().Get("X-Request-ID")
		assert.NotEmpty(t, id)
		ids[id] = true
	}
	assert.Len(t, ids, 10, "each request must get a unique ID")
}

func TestRequestID_PropagatedID_EchoedInResponse(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "client-provided-id-xyz")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, "client-provided-id-xyz", rec.Header().Get("X-Request-ID"))
}

// ── Rate limit + X-Forwarded-For ─────────────────────────────────────────────

func TestRateLimitByIP_XForwardedFor_UsedAsKey(t *testing.T) {
	const rpm = 2
	r := chi.NewRouter()
	r.Use(middleware.RateLimitByIP(rpm))
	r.Get("/", okHandler)

	// Exhaust quota for the forwarded IP
	for i := 0; i < rpm; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		req.RemoteAddr = "10.0.0.1:1234" // different "real" IP
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	// Rate-limited based on forwarded IP
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestRateLimitByUser_UnauthenticatedRequest_FallsBackToIP(t *testing.T) {
	const rpm = 2
	r := chi.NewRouter()
	// No claims middleware — unauthenticated
	r.Use(middleware.RateLimitByUser(rpm))
	r.Get("/", okHandler)

	// First rpm requests from same IP should pass
	for i := 0; i < rpm; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.50:1234"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d should pass", i+1)
	}

	// (rpm+1)th must be blocked
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.50:1234"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "unauthenticated client must be IP-limited")
}

func TestRateLimitByIP_RetryAfterHeader_Is60(t *testing.T) {
	const rpm = 1
	r := chi.NewRouter()
	r.Use(middleware.RateLimitByIP(rpm))
	r.Get("/", okHandler)

	// Exhaust the one allowed request
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "10.10.10.10:9999"
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)

	// Next one is rate-limited
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.10.10.10:9999"
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	assert.Equal(t, "60", rec2.Header().Get("Retry-After"))
}

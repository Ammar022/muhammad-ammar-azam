package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/middleware"
)

// okHandler is a minimal HTTP handler that always returns 200 OK.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
})

// ── SecureHeaders middleware tests ────────────────────────────────────────────

func TestSecureHeaders_ArePresentOnEveryResponse(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.SecureHeaders)
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	headers := rec.Header()
	assert.Equal(t, "nosniff", headers.Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", headers.Get("X-Frame-Options"))
	assert.Equal(t, "1; mode=block", headers.Get("X-XSS-Protection"))
	assert.NotEmpty(t, headers.Get("Content-Security-Policy"))
	assert.NotEmpty(t, headers.Get("Strict-Transport-Security"))
	assert.Equal(t, "no-store", headers.Get("Cache-Control"))
}

// ── RequestSizeLimit middleware tests ─────────────────────────────────────────

func TestRequestSizeLimit_RejectsOversizedBody(t *testing.T) {
	const maxBytes int64 = 100

	r := chi.NewRouter()
	r.Use(middleware.RequestSizeLimit(maxBytes))
	r.Post("/", okHandler)

	// Body larger than limit
	body := strings.NewReader(strings.Repeat("x", 200))
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestRequestSizeLimit_AllowsSmallBody(t *testing.T) {
	const maxBytes int64 = 1024

	r := chi.NewRouter()
	r.Use(middleware.RequestSizeLimit(maxBytes))
	r.Post("/", okHandler)

	body := strings.NewReader(`{"question":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ── RequireJSON middleware tests ──────────────────────────────────────────────

func TestRequireJSON_RejectsMissingContentType(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Post("/", okHandler)

	body := strings.NewReader(`{"question":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	// No Content-Type header set
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
}

func TestRequireJSON_AcceptsApplicationJSON(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Post("/", okHandler)

	body := strings.NewReader(`{"question":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireJSON_AllowsGET_WithoutContentType(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequireJSON)
	r.Get("/", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// GET requests don't need a Content-Type
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ── AntiReplay middleware tests ───────────────────────────────────────────────

func TestAntiReplay_RejectsMissingTimestamp(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Nonce", "abc123")
	// Missing X-Request-Timestamp
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAntiReplay_RejectsMissingNonce(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	// Missing X-Nonce
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAntiReplay_RejectsStaleTimestamp(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300)) // 300s window
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	// Timestamp 10 minutes in the past — outside the 5-minute window
	stale := time.Now().Add(-10 * time.Minute).Unix()
	req.Header.Set("X-Request-Timestamp", fmt.Sprintf("%d", stale))
	req.Header.Set("X-Nonce", "stale-nonce-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAntiReplay_AcceptsValidRequest(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("X-Nonce", "valid-unique-nonce-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAntiReplay_RejectsReplayedNonce(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	nonce := "replay-nonce-unique-99"
	ts := fmt.Sprintf("%d", time.Now().Unix())

	// First request — must succeed
	req1 := httptest.NewRequest(http.MethodPost, "/", nil)
	req1.Header.Set("X-Request-Timestamp", ts)
	req1.Header.Set("X-Nonce", nonce)
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code, "first request should succeed")

	// Second request with same nonce — must be rejected (replay)
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set("X-Request-Timestamp", ts)
	req2.Header.Set("X-Nonce", nonce)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusUnauthorized, rec2.Code, "replayed nonce should be rejected")
}

// ── Timeout middleware tests ──────────────────────────────────────────────────

func TestTimeout_ReturnsGatewayTimeoutWhenHandlerIsSlow(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.Timeout(50 * time.Millisecond)) // very short timeout for testing
	r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until context is cancelled
		<-r.Context().Done()
		// Only write if context wasn't cancelled (i.e., normal completion)
		select {
		case <-r.Context().Done():
			// Context cancelled — don't write anything
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
}

// ── RequestID middleware tests ────────────────────────────────────────────────

func TestRequestID_GeneratesID(t *testing.T) {
	var captured string
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = middleware.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.NotEmpty(t, captured, "request ID should be injected into context")
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"), "request ID should appear in response header")
}

func TestRequestID_PropagatesExistingID(t *testing.T) {
	existingID := "my-custom-request-id-123"
	var captured string

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = middleware.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existingID)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, existingID, captured)
	assert.Equal(t, existingID, rec.Header().Get("X-Request-ID"))
}

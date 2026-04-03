package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	"github.com/Ammar022/muhammad-ammar-azam/internal/shared/middleware"
)

// ── Timestamp edge cases ──────────────────────────────────────────────────────

func TestAntiReplay_RejectsNonNumericTimestamp(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", "not-a-number")
	req.Header.Set("X-Nonce", "abc")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAntiReplay_RejectsFloatTimestamp(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", "1711234567.99")
	req.Header.Set("X-Nonce", "abc")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAntiReplay_RejectsFutureTimestamp(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300)) // ±300s window
	r.Post("/", okHandler)

	future := time.Now().Add(10 * time.Minute).Unix() // 10 min in future — outside window
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", fmt.Sprintf("%d", future))
	req.Header.Set("X-Nonce", "future-nonce-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAntiReplay_AcceptsFutureTimestampWithinWindow(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	// 1 minute in future — within the ±300s window
	future := time.Now().Add(1 * time.Minute).Unix()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", fmt.Sprintf("%d", future))
	req.Header.Set("X-Nonce", "near-future-nonce-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAntiReplay_RejectsNegativeTimestamp(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", "-1")
	req.Header.Set("X-Nonce", "abc")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAntiReplay_RejectsZeroTimestamp(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", "0")
	req.Header.Set("X-Nonce", "abc")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// ── Nonce edge cases ──────────────────────────────────────────────────────────

func TestAntiReplay_RejectsEmptyNonce(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("X-Nonce", "") // present but empty
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAntiReplay_AcceptsSpecialCharactersInNonce(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	req.Header.Set("X-Nonce", "nonce-abc-XYZ-123-!@#")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAntiReplay_TwoDistinctNonces_BothAccepted(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	ts := fmt.Sprintf("%d", time.Now().Unix())

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("X-Request-Timestamp", ts)
		req.Header.Set("X-Nonce", fmt.Sprintf("unique-nonce-distinct-%d", i))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code, "request %d with unique nonce should succeed", i)
	}
}

// ── GET requests pass through ─────────────────────────────────────────────────

func TestAntiReplay_AppliesToGetRequests(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Get("/", okHandler)

	// GET without headers → missing security header
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// AntiReplay applies to ALL methods, not just POST
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ── Nonce cache concurrency ───────────────────────────────────────────────────

func TestNonceCache_Seen_ConcurrentCalls_NoDuplicates(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	const goroutines = 50

	results := make([]bool, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			results[i] = cache.Seen("shared-nonce")
		}(i)
	}
	wg.Wait()

	// Exactly one goroutine should see it as NOT seen (the first one to insert)
	firstSeen := 0
	alreadySeen := 0
	for _, seen := range results {
		if seen {
			alreadySeen++
		} else {
			firstSeen++
		}
	}
	assert.Equal(t, 1, firstSeen, "exactly one goroutine should be the first to register the nonce")
	assert.Equal(t, goroutines-1, alreadySeen)
}

func TestNonceCache_UniqueNonces_NeverCollide(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	const count = 100

	for i := 0; i < count; i++ {
		nonce := fmt.Sprintf("unique-nonce-%d", i)
		seen := cache.Seen(nonce)
		assert.False(t, seen, "nonce %s should not have been seen before", nonce)
	}
}

func TestNonceCache_SameNonceTwice_SecondSeenIsTrue(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	nonce := "replay-test-nonce"

	assert.False(t, cache.Seen(nonce))
	assert.True(t, cache.Seen(nonce))
	assert.True(t, cache.Seen(nonce)) // third call also true
}

// ── Both headers missing ──────────────────────────────────────────────────────

func TestAntiReplay_BothHeadersMissing_Returns400(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ── Window boundary test ──────────────────────────────────────────────────────

func TestAntiReplay_TimestampJustOutsideWindow_Rejected(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 60)) // 60s window
	r.Post("/", okHandler)

	// 61s in the past — just outside the window
	ts := time.Now().Add(-61 * time.Second).Unix()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", fmt.Sprintf("%d", ts))
	req.Header.Set("X-Nonce", "boundary-nonce-1")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAntiReplay_TimestampJustInsideWindow_Accepted(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 60))
	r.Post("/", okHandler)

	// 59s in the past — just inside the window
	ts := time.Now().Add(-59 * time.Second).Unix()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-Request-Timestamp", fmt.Sprintf("%d", ts))
	req.Header.Set("X-Nonce", "boundary-nonce-inside")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

// ── Parallel anti-replay requests ────────────────────────────────────────────

func TestAntiReplay_ConcurrentDistinctNonces_AllPass(t *testing.T) {
	cache := middleware.NewNonceCache(5 * time.Minute)
	r := chi.NewRouter()
	r.Use(middleware.AntiReplay(cache, 300))
	r.Post("/", okHandler)

	ts := fmt.Sprintf("%d", time.Now().Unix())
	const goroutines = 20
	codes := make([]int, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set("X-Request-Timestamp", ts)
			req.Header.Set("X-Nonce", fmt.Sprintf("parallel-nonce-%d", i))
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()

	for i, code := range codes {
		assert.Equal(t, http.StatusOK, code, "goroutine %d should succeed with unique nonce", i)
	}
}

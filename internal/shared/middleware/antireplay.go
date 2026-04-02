package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/response"
)

// nonceEntry holds the expiry time for a seen nonce value.
type nonceEntry struct {
	expiresAt time.Time
}

// NonceCache is a thread-safe, in-memory store of recently seen nonces.
// In a multi-instance deployment this should be backed by a shared Redis cache.
// For single-instance deployments or tests, the in-memory version is sufficient.
//
// A background goroutine periodically evicts expired entries to prevent
// unbounded memory growth.
type NonceCache struct {
	mu      sync.RWMutex
	entries map[string]nonceEntry
	ttl     time.Duration
}

// NewNonceCache creates a NonceCache with the given TTL and starts the cleanup
// goroutine.  TTL should be at least 2× the anti-replay window so every nonce
// seen within the window is remembered until after the window passes.
func NewNonceCache(ttl time.Duration) *NonceCache {
	nc := &NonceCache{
		entries: make(map[string]nonceEntry),
		ttl:     ttl,
	}
	go nc.cleanupLoop()
	return nc
}

// Seen returns true if the nonce has been seen before (replay detected).
// If not seen, it records the nonce and returns false.
func (nc *NonceCache) Seen(nonce string) bool {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if _, exists := nc.entries[nonce]; exists {
		return true // already seen → replay
	}

	// Record the nonce with an expiry
	nc.entries[nonce] = nonceEntry{expiresAt: time.Now().Add(nc.ttl)}
	return false
}

// cleanupLoop evicts expired nonces every minute to keep memory bounded.
func (nc *NonceCache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		nc.evictExpired()
	}
}

func (nc *NonceCache) evictExpired() {
	now := time.Now()
	nc.mu.Lock()
	defer nc.mu.Unlock()
	for k, v := range nc.entries {
		if now.After(v.expiresAt) {
			delete(nc.entries, k)
		}
	}
}

// AntiReplay returns middleware implementing timestamp + nonce validation.
//
// Every authenticated request MUST include:
//   - X-Request-Timestamp: Unix timestamp (seconds) — must be within ±windowSec of server time
//   - X-Nonce: a unique string per request — rejected if seen before within the window
//
// This two-factor approach prevents replay attacks: an attacker cannot reuse a
// captured request because:
//  1. A fresh timestamp would differ, and
//  2. A new nonce would be required (the old one is recorded as used).
//
// Why this pattern?  Token possession alone is not sufficient — a stolen
// bearer token could be replayed indefinitely until expiry.  Nonce + timestamp
// binding limits the replay window to a few minutes maximum.
func AntiReplay(cache *NonceCache, windowSec int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// ── 1. Validate timestamp ──────────────────────────────────────────
			tsHeader := r.Header.Get("X-Request-Timestamp")
			if tsHeader == "" {
				response.Error(w, apperrors.ErrMissingSecurityHeader)
				return
			}

			tsUnix, err := strconv.ParseInt(tsHeader, 10, 64)
			if err != nil {
				response.Error(w, apperrors.ErrTimestampInvalid)
				return
			}

			requestTime := time.Unix(tsUnix, 0)
			window := time.Duration(windowSec) * time.Second
			now := time.Now()

			if requestTime.Before(now.Add(-window)) || requestTime.After(now.Add(window)) {
				response.Error(w, apperrors.ErrTimestampInvalid)
				return
			}

			// ── 2. Validate nonce ──────────────────────────────────────────────
			nonce := r.Header.Get("X-Nonce")
			if nonce == "" {
				response.Error(w, apperrors.ErrMissingSecurityHeader)
				return
			}

			if cache.Seen(nonce) {
				response.Error(w, apperrors.ErrReplayDetected)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

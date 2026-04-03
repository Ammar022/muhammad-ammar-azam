package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	apperrors "github.com/Ammar022/muhammad-ammar-azam/internal/shared/errors"
	"github.com/Ammar022/muhammad-ammar-azam/internal/shared/response"
)

type nonceEntry struct {
	expiresAt time.Time
}

type NonceCache struct {
	mu      sync.RWMutex
	entries map[string]nonceEntry
	ttl     time.Duration
}

func NewNonceCache(ttl time.Duration) *NonceCache {
	nc := &NonceCache{
		entries: make(map[string]nonceEntry),
		ttl:     ttl,
	}
	go nc.cleanupLoop()
	return nc
}

func (nc *NonceCache) Seen(nonce string) bool {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if _, exists := nc.entries[nonce]; exists {
		return true
	}

	nc.entries[nonce] = nonceEntry{expiresAt: time.Now().Add(nc.ttl)}
	return false
}

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

func AntiReplay(cache *NonceCache, windowSec int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/auth"
	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/response"
)

// visitor tracks the rate limiter and last-seen time for a single client.
// The last-seen time allows the cleanup goroutine to evict idle visitors.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// rateLimiterStore is a thread-safe map of limiters keyed by client identity.
type rateLimiterStore struct {
	mu       sync.RWMutex
	visitors map[string]*visitor
	rpm      int // requests per minute
}

// newRateLimiterStore creates a store and starts the cleanup goroutine.
func newRateLimiterStore(rpm int) *rateLimiterStore {
	s := &rateLimiterStore{
		visitors: make(map[string]*visitor),
		rpm:      rpm,
	}
	go s.cleanupLoop()
	return s
}

// getLimiter retrieves or creates a token-bucket limiter for the given key.
// The bucket holds burst=rpm tokens and refills at rpm/60 tokens per second,
// which approximates a per-minute limit while smoothing short bursts.
func (s *rateLimiterStore) getLimiter(key string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, exists := s.visitors[key]
	if !exists {
		// Convert RPM to per-second rate; allow a burst equal to the RPM
		r := rate.Every(time.Minute / time.Duration(s.rpm))
		v = &visitor{
			limiter:  rate.NewLimiter(r, s.rpm),
			lastSeen: time.Now(),
		}
		s.visitors[key] = v
	}

	v.lastSeen = time.Now()
	return v.limiter
}

// cleanupLoop evicts visitors that have been idle for more than 3 minutes.
func (s *rateLimiterStore) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		for k, v := range s.visitors {
			if time.Since(v.lastSeen) > 3*time.Minute {
				delete(s.visitors, k)
			}
		}
		s.mu.Unlock()
	}
}

// ── Middleware constructors ───────────────────────────────────────────────────

// RateLimitByIP returns middleware that limits requests per source IP address.
// This protects unauthenticated endpoints (auth routes, health check) from
// brute-force and enumeration attacks.
func RateLimitByIP(rpm int) func(http.Handler) http.Handler {
	store := newRateLimiterStore(rpm)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			if !store.getLimiter(ip).Allow() {
				w.Header().Set("Retry-After", "60")
				response.Error(w, apperrors.ErrRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitByUser returns middleware that limits requests per authenticated user.
// It falls back to per-IP limiting for unauthenticated requests, ensuring that
// anonymous callers cannot exhaust per-user quotas.
//
// This middleware must be placed AFTER JWT validation so auth.ClaimsFromContext
// can retrieve the user identity.
func RateLimitByUser(rpm int) func(http.Handler) http.Handler {
	store := newRateLimiterStore(rpm)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var key string
			if claims, ok := auth.ClaimsFromContext(r.Context()); ok {
				// Limit by Auth0 subject (stable, unique per user)
				key = "user:" + claims.Subject
			} else {
				// Unauthenticated — fall back to IP
				key = "ip:" + realIP(r)
			}

			if !store.getLimiter(key).Allow() {
				w.Header().Set("Retry-After", "60")
				response.Error(w, apperrors.ErrRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// realIP extracts the real client IP, preferring X-Forwarded-For when the
// application is behind a trusted proxy (load balancer, API gateway).
// Falls back to the direct connection's remote address.
//
// Security note: X-Forwarded-For can be spoofed by clients.  In a real
// production setup, strip all but the last trusted proxy-added hop.
func realIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first (leftmost) address which is the original client
		if idx := len(xff); idx > 0 {
			if ip := xff[:idx]; ip != "" {
				if host, _, err := net.SplitHostPort(ip); err == nil {
					return host
				}
				return ip
			}
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

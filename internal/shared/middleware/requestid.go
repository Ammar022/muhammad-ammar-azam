// Package middleware contains all HTTP middleware used across the application.
// Each middleware addresses a single concern (request IDs, security headers,
// rate limiting, etc.) to keep them composable and testable in isolation.
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// requestIDKey is the context key for the request ID value.
type requestIDKey struct{}

const (
	// RequestIDHeader is the HTTP header used to propagate request IDs.
	// Clients may provide their own; the server generates one if absent.
	RequestIDHeader = "X-Request-ID"
)

// RequestID is middleware that ensures every request has a unique ID.
// It reads X-Request-ID from the incoming request (useful when a gateway
// sets it upstream), or generates a fresh UUID v4 otherwise.
//
// The ID is:
//   - Stored in the request context (retrieve with RequestIDFromContext)
//   - Echoed back in the response via the X-Request-ID header
//   - Included in all structured log entries via the logging middleware
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = uuid.New().String()
		}

		// Propagate the ID in the response so clients can correlate logs
		w.Header().Set(RequestIDHeader, id)

		// Store in context for downstream use
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext retrieves the request ID from context.
// Returns an empty string if not present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

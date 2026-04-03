package middleware

import (
	"net/http"
	"strings"

	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/response"
)

func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		// Prevent browsers from MIME-sniffing a response away from the declared content-type
		h.Set("X-Content-Type-Options", "nosniff")

		// Deny framing entirely to prevent clickjacking
		h.Set("X-Frame-Options", "DENY")

		// Force HTTPS for one year (includeSubDomains for completeness)
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Enable the XSS filter in older browsers; modern ones use CSP instead
		h.Set("X-XSS-Protection", "1; mode=block")

		// Strict referrer policy: do not leak URL to cross-origin destinations
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrictive Content Security Policy for an API (no HTML content served)
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

		// Disable all browser features/APIs not needed by a REST API
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Prevent caching of sensitive API responses
		h.Set("Cache-Control", "no-store")

		// Remove server identification to reduce reconnaissance surface
		h.Del("X-Powered-By")
		h.Set("Server", "")

		next.ServeHTTP(w, r)
	})
}

// RequestSizeLimit returns middleware that rejects request bodies exceeding
// maxBytes.  Without this, malicious clients can send multi-GB payloads and
// exhaust server memory
func RequestSizeLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Reject based on Content-Length header (fast path, client-declared)
			if r.ContentLength > maxBytes {
				response.Error(w, apperrors.ErrBodyTooLarge)
				return
			}

			// Enforce at the reader level so even streaming uploads are capped
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// RequireJSON rejects any non-GET/HEAD/OPTIONS request that does not declare
// Content-Type: application/json.  This prevents form-based CSRF attacks and
// ensures the body parser never processes unexpected content formats.
func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only enforce on methods that carry a request body
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			ct := r.Header.Get("Content-Type")
			if !strings.Contains(ct, "application/json") {
				response.Error(w, apperrors.ErrContentType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/auth"
)

// responseRecorder wraps http.ResponseWriter to capture the status code after
// the handler writes it.  We need this because http.ResponseWriter does not
// expose the status code after WriteHeader is called.
type responseRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (rr *responseRecorder) WriteHeader(status int) {
	rr.status = status
	rr.ResponseWriter.WriteHeader(status)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	n, err := rr.ResponseWriter.Write(b)
	rr.size += n
	return n, err
}

// Logger returns structured request/response logging middleware using zerolog.
// Every request emits a log entry with:
//   - request_id  – from X-Request-ID context value
//   - user_id     – Auth0 subject (if authenticated)
//   - method, path, remote_ip
//   - status code, response size
//   - latency (ms) – response time
//
// This provides the observability baseline required by the specification.
func Logger(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rr := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rr, r)

			latency := time.Since(start)

			// Build the log event with all standard fields
			event := logger.Info().
				Str("request_id", RequestIDFromContext(r.Context())).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remote_ip", realIP(r)).
				Int("status", rr.status).
				Int("bytes", rr.size).
				Dur("latency_ms", latency)

			// Include user ID when the request is authenticated
			if claims, ok := auth.ClaimsFromContext(r.Context()); ok {
				event = event.Str("user_id", claims.Subject)
			}

			// Downgrade 4xx to warn and 5xx to error so alerts are meaningful
			switch {
			case rr.status >= 500:
				event.Msg("server error")
			case rr.status >= 400:
				log.Warn().
					Str("request_id", RequestIDFromContext(r.Context())).
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Int("status", rr.status).
					Dur("latency_ms", latency).
					Msg("client error")
			default:
				event.Msg("request")
			}
		})
	}
}

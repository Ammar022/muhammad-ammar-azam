package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/auth"
)

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

func Logger(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			rr := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rr, r)

			latency := time.Since(start)

			event := logger.Info().
				Str("request_id", RequestIDFromContext(r.Context())).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Str("remote_ip", realIP(r)).
				Int("status", rr.status).
				Int("bytes", rr.size).
				Dur("latency_ms", latency)

			if claims, ok := auth.ClaimsFromContext(r.Context()); ok {
				event = event.Str("user_id", claims.Subject)
			}

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

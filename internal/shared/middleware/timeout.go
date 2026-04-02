package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/response"
)

// Timeout returns middleware that cancels the request context after the given
// duration.  Handlers that respect context cancellation will abort early,
// preventing resource leaks from slow clients or downstream dependencies.
//
// Note: once WriteHeader has been called by the handler, we cannot change
// the status code.  The timeout check only kicks in if the context is
// cancelled before the handler writes.  For long-running handlers, use
// context.Done() checks inside the handler itself.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()

			// Channel to signal that the handler completed normally
			done := make(chan struct{}, 1)

			// Wrap the writer to prevent writing after timeout
			tw := &timeoutWriter{ResponseWriter: w}

			go func() {
				next.ServeHTTP(tw, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				// Handler finished in time — nothing to do

			case <-ctx.Done():
				tw.mu.Lock()
				defer tw.mu.Unlock()

				// Only respond with timeout if the handler has not already written headers
				if !tw.wroteHeader {
					tw.timedOut = true // prevent the handler goroutine from writing too
					response.Error(w, apperrors.Wrap(
						http.StatusGatewayTimeout,
						"REQUEST_TIMEOUT",
						"Request timed out",
						ctx.Err(),
					))
				}
			}
		})
	}
}

// timeoutWriter wraps ResponseWriter and prevents writes after the timeout
// fires to avoid "superfluous response.WriteHeader call" panics.
//
// Two separate flags are used:
//   - wroteHeader: set when WriteHeader is called normally (allows body writes to continue)
//   - timedOut: set only when the timeout fires (blocks all further writes)
type timeoutWriter struct {
	http.ResponseWriter
	mu          sync.Mutex
	wroteHeader bool
	timedOut    bool
}

func (tw *timeoutWriter) WriteHeader(status int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if !tw.timedOut && !tw.wroteHeader {
		tw.wroteHeader = true
		tw.ResponseWriter.WriteHeader(status)
	}
}

func (tw *timeoutWriter) Write(b []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.timedOut {
		return 0, nil
	}
	return tw.ResponseWriter.Write(b)
}

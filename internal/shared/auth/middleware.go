package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/rs/zerolog/log"

	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/config"
	apperrors "github.com/Ammar022/secure-ai-chat-backend/internal/shared/errors"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/response"
)

// Validator validates Auth0 JWTs using a cached JWKS key set.
// It enforces issuer, audience, expiry, and signature validation.
//
// Design note: We cache the JWKS with auto-refresh to avoid fetching
// Auth0's public keys on every request while staying resilient to key rotation.
type Validator struct {
	cfg      config.Auth0Config
	keyCache *jwk.Cache
}

// NewValidator creates a Validator and warms up the JWKS cache.
// This should be called once at application startup.
// AUTH0_DOMAIN must be set; the application will not start without it.
func NewValidator(ctx context.Context, cfg config.Auth0Config) (*Validator, error) {
	if cfg.Domain == "" {
		return nil, fmt.Errorf("auth: AUTH0_DOMAIN is required")
	}

	cache := jwk.NewCache(ctx)

	// Register the JWKS endpoint with auto-refresh interval of 15 minutes.
	// The library will refresh the cache in the background before keys expire.
	if err := cache.Register(cfg.JWKSEndpoint(), jwk.WithMinRefreshInterval(15*time.Minute)); err != nil {
		return nil, fmt.Errorf("auth: register JWKS cache: %w", err)
	}

	// Eagerly fetch the keys so the first request does not incur JWKS latency
	if _, err := cache.Refresh(ctx, cfg.JWKSEndpoint()); err != nil {
		return nil, fmt.Errorf("auth: initial JWKS fetch failed: %w", err)
	}

	return &Validator{cfg: cfg, keyCache: cache}, nil
}

// Middleware validates the Auth0 Bearer token on every request.
// It extracts the token, validates it against the Auth0 JWKS (RS256),
// and stores the parsed claims in the request context.
func (v *Validator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr, err := extractBearerToken(r)
		if err != nil {
			response.Error(w, apperrors.ErrUnauthorized)
			return
		}

		keySet, err := v.keyCache.Get(r.Context(), v.cfg.JWKSEndpoint())
		if err != nil {
			log.Error().Err(err).Msg("auth: failed to retrieve JWKS from cache")
			response.Error(w, apperrors.ErrInternal)
			return
		}

		token, parseErr := jwt.Parse(
			[]byte(tokenStr),
			jwt.WithKeySet(keySet),
			jwt.WithValidate(true),
			jwt.WithIssuer(v.cfg.Issuer()),
			jwt.WithAudience(v.cfg.Audience),
			jwt.WithAcceptableSkew(30*time.Second),
		)
		if parseErr != nil {
			if strings.Contains(parseErr.Error(), "exp not satisfied") {
				response.Error(w, apperrors.ErrTokenExpired)
				return
			}
			log.Warn().Err(parseErr).Msg("auth: JWT validation failed")
			response.Error(w, apperrors.ErrTokenInvalid)
			return
		}

		claims, err := v.extractClaims(token)
		if err != nil {
			log.Warn().Err(err).Msg("auth: Auth0 claims extraction failed")
			response.Error(w, apperrors.ErrTokenInvalid)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
	})
}

// RequireRole returns middleware that enforces a specific role.
// It must be chained AFTER the JWT validation middleware.
//
// This enforces RBAC at the controller level (domain policies add a second
// layer of enforcement for sensitive operations).
func RequireRole(roles ...Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				response.Error(w, apperrors.ErrUnauthorized)
				return
			}

			for _, requiredRole := range roles {
				if claims.HasRole(requiredRole) {
					next.ServeHTTP(w, r)
					return
				}
			}

			response.Error(w, apperrors.ErrForbidden)
		})
	}
}

// extractBearerToken parses "Authorization: Bearer <token>" from the request.
func extractBearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", fmt.Errorf("missing Authorization header")
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", fmt.Errorf("invalid Authorization header format")
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", fmt.Errorf("empty Bearer token")
	}

	return token, nil
}

// extractClaims maps raw JWT claims to our typed Claims struct.
// Roles are extracted from the custom Auth0 namespace claim.
func (v *Validator) extractClaims(token jwt.Token) (*Claims, error) {
	subject := token.Subject()
	if subject == "" {
		return nil, fmt.Errorf("token missing subject claim")
	}

	// Extract email from standard claim (Auth0 sets this if the scope includes "email")
	email, _ := token.Get("email")
	emailStr, _ := email.(string)

	// Extract custom roles claim injected by Auth0 Action/Rule
	// Expected value: []interface{} containing role strings
	var roles []Role
	if rawRoles, ok := token.Get(v.cfg.RolesClaim); ok {
		if roleSlice, ok := rawRoles.([]interface{}); ok {
			for _, r := range roleSlice {
				if roleStr, ok := r.(string); ok {
					roles = append(roles, Role(roleStr))
				}
			}
		}
	}

	// Default to "user" role if no roles are present in the token
	if len(roles) == 0 {
		roles = []Role{RoleUser}
	}

	return &Claims{
		Subject: subject,
		Email:   emailStr,
		Roles:   roles,
	}, nil
}

package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds every setting the application needs.  All fields are populated
// from environment variables; see .env.example for the full list.
type Config struct {
	App       AppConfig
	DB        DBConfig
	Auth0     Auth0Config
	Security  SecurityConfig
	RateLimit RateLimitConfig
	AI        AIConfig
	Log       LogConfig
}

// AppConfig holds generic application-level settings.
type AppConfig struct {
	Env                    string // development | staging | production
	Port                   int
	Name                   string
	Version                string
	RenewalIntervalMinutes int // how often the renewal job runs (default 60)
}

// DBConfig holds PostgreSQL connection parameters.
type DBConfig struct {
	Host            string
	Port            int
	Name            string
	User            string
	Password        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DSN returns the PostgreSQL data-source name (connection string).
func (d DBConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		d.Host, d.Port, d.Name, d.User, d.Password, d.SSLMode,
	)
}

// Auth0Config holds settings for the external OIDC/OAuth2 provider (Auth0).
// The backend validates JWTs issued by Auth0 – it never issues tokens itself.
type Auth0Config struct {
	Domain     string // e.g. "your-tenant.auth0.com"
	Audience   string // API identifier configured in the Auth0 dashboard
	RolesClaim string // custom namespace for the roles array claim
}

// JWKSEndpoint returns the well-known JWKS URL for Auth0.
// If Domain is already a full URL (starts with http:// or https://) it is
// used directly — this allows integration tests to point at an httptest server.
func (a Auth0Config) JWKSEndpoint() string {
	if strings.HasPrefix(a.Domain, "http://") || strings.HasPrefix(a.Domain, "https://") {
		return a.Domain + "/.well-known/jwks.json"
	}
	return fmt.Sprintf("https://%s/.well-known/jwks.json", a.Domain)
}

// Issuer returns the expected token issuer URL.
func (a Auth0Config) Issuer() string {
	if strings.HasPrefix(a.Domain, "http://") || strings.HasPrefix(a.Domain, "https://") {
		return a.Domain + "/"
	}
	return fmt.Sprintf("https://%s/", a.Domain)
}

// SecurityConfig holds all security-related settings.
type SecurityConfig struct {
	CORSAllowedOrigins  []string
	AntiReplayWindowSec int
	MaxRequestBodyBytes int64
	RequestTimeoutSec   int
}

// RateLimitConfig holds per-endpoint rate limits (requests per minute).
type RateLimitConfig struct {
	IPRPM           int // global per-IP
	UserRPM         int // per authenticated user
	AuthRPM         int // /auth/* endpoints
	ChatRPM         int // /chat/* endpoints
	SubscriptionRPM int // /subscription/* endpoints
}

// AIConfig holds parameters for the mocked AI service.
type AIConfig struct {
	LatencyMinMs int
	LatencyMaxMs int
}

type LogConfig struct {
	Level  string // debug | info | warn | error
	Format string // json | console
}

func Load() (*Config, error) {
	v := viper.New()

	v.SetConfigFile(".env")
	v.SetConfigType("env")
	_ = v.ReadInConfig() 

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	v.SetDefault("APP_ENV", "development")
	v.SetDefault("APP_PORT", 8080)
	v.SetDefault("APP_NAME", "secure-ai-chat-backend")
	v.SetDefault("APP_VERSION", "1.0.0")

	v.SetDefault("DB_PORT", 5432)
	v.SetDefault("DB_SSLMODE", "disable")
	v.SetDefault("DB_MAX_OPEN_CONNS", 25)
	v.SetDefault("DB_MAX_IDLE_CONNS", 5)
	v.SetDefault("DB_CONN_MAX_LIFETIME", 300)

	v.SetDefault("AUTH0_ROLES_CLAIM", "https://api.yourdomain.com/roles")

	v.SetDefault("CORS_ALLOWED_ORIGINS", "http://localhost:3000")
	v.SetDefault("ANTI_REPLAY_WINDOW_SECONDS", 300)
	v.SetDefault("MAX_REQUEST_BODY_BYTES", 1048576)
	v.SetDefault("REQUEST_TIMEOUT_SECONDS", 30)

	v.SetDefault("RATE_LIMIT_IP_RPM", 100)
	v.SetDefault("RATE_LIMIT_USER_RPM", 50)
	v.SetDefault("RATE_LIMIT_AUTH_RPM", 10)
	v.SetDefault("RATE_LIMIT_CHAT_RPM", 20)
	v.SetDefault("RATE_LIMIT_SUBSCRIPTION_RPM", 30)

	v.SetDefault("AI_LATENCY_MIN_MS", 500)
	v.SetDefault("AI_LATENCY_MAX_MS", 2000)

	v.SetDefault("RENEWAL_INTERVAL_MINUTES", 60)

	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_FORMAT", "json")

	required := []string{"DB_HOST", "DB_NAME", "DB_USER", "DB_PASSWORD", "AUTH0_DOMAIN", "AUTH0_AUDIENCE"}
	for _, key := range required {
		if v.GetString(key) == "" {
			return nil, fmt.Errorf("config: required environment variable %q is not set", key)
		}
	}

	cfg := &Config{
		App: AppConfig{
			Env:                    v.GetString("APP_ENV"),
			Port:                   v.GetInt("APP_PORT"),
			Name:                   v.GetString("APP_NAME"),
			Version:                v.GetString("APP_VERSION"),
			RenewalIntervalMinutes: v.GetInt("RENEWAL_INTERVAL_MINUTES"),
		},
		DB: DBConfig{
			Host:            v.GetString("DB_HOST"),
			Port:            v.GetInt("DB_PORT"),
			Name:            v.GetString("DB_NAME"),
			User:            v.GetString("DB_USER"),
			Password:        v.GetString("DB_PASSWORD"),
			SSLMode:         v.GetString("DB_SSLMODE"),
			MaxOpenConns:    v.GetInt("DB_MAX_OPEN_CONNS"),
			MaxIdleConns:    v.GetInt("DB_MAX_IDLE_CONNS"),
			ConnMaxLifetime: time.Duration(v.GetInt("DB_CONN_MAX_LIFETIME")) * time.Second,
		},
		Auth0: Auth0Config{
			Domain:     v.GetString("AUTH0_DOMAIN"),
			Audience:   v.GetString("AUTH0_AUDIENCE"),
			RolesClaim: v.GetString("AUTH0_ROLES_CLAIM"),
		},
		Security: SecurityConfig{
			CORSAllowedOrigins:  strings.Split(v.GetString("CORS_ALLOWED_ORIGINS"), ","),
			AntiReplayWindowSec: v.GetInt("ANTI_REPLAY_WINDOW_SECONDS"),
			MaxRequestBodyBytes: v.GetInt64("MAX_REQUEST_BODY_BYTES"),
			RequestTimeoutSec:   v.GetInt("REQUEST_TIMEOUT_SECONDS"),
		},
		RateLimit: RateLimitConfig{
			IPRPM:           v.GetInt("RATE_LIMIT_IP_RPM"),
			UserRPM:         v.GetInt("RATE_LIMIT_USER_RPM"),
			AuthRPM:         v.GetInt("RATE_LIMIT_AUTH_RPM"),
			ChatRPM:         v.GetInt("RATE_LIMIT_CHAT_RPM"),
			SubscriptionRPM: v.GetInt("RATE_LIMIT_SUBSCRIPTION_RPM"),
		},
		AI: AIConfig{
			LatencyMinMs: v.GetInt("AI_LATENCY_MIN_MS"),
			LatencyMaxMs: v.GetInt("AI_LATENCY_MAX_MS"),
		},
		Log: LogConfig{
			Level:  v.GetString("LOG_LEVEL"),
			Format: v.GetString("LOG_FORMAT"),
		},
	}

	return cfg, nil
}

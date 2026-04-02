// @title				Secure AI Chat Backend
// @version			1.0
// @description		Production-grade AI chat backend with Auth0 authentication, DDD, quota management, and subscription bundles.
// @host				localhost:8080
// @BasePath			/
// @securityDefinitions.apikey	BearerAuth
// @in					header
// @name				Authorization
// @description		Auth0 JWT — prefix with "Bearer ". Obtain via Auth0 dashboard (APIs → Test tab) or your Auth0 login flow.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog/log"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/Ammar022/secure-ai-chat-backend/docs"

	adminctrl "github.com/Ammar022/secure-ai-chat-backend/internal/admin/controller"
	authctrl "github.com/Ammar022/secure-ai-chat-backend/internal/auth/controller"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/auth"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/config"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/database"
	applogger "github.com/Ammar022/secure-ai-chat-backend/internal/shared/logger"
	"github.com/Ammar022/secure-ai-chat-backend/internal/shared/middleware"

	chatctrl "github.com/Ammar022/secure-ai-chat-backend/internal/chat/controller"
	chatdomain "github.com/Ammar022/secure-ai-chat-backend/internal/chat/domain"
	chatrepo "github.com/Ammar022/secure-ai-chat-backend/internal/chat/repository"

	subctrl "github.com/Ammar022/secure-ai-chat-backend/internal/subscription/controller"
	subdomain "github.com/Ammar022/secure-ai-chat-backend/internal/subscription/domain"
	subrepo "github.com/Ammar022/secure-ai-chat-backend/internal/subscription/repository"

	userrepo "github.com/Ammar022/secure-ai-chat-backend/internal/user/repository"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: config load failed: %v\n", err)
		os.Exit(1)
	}

	logger := applogger.New(cfg.Log.Level, cfg.Log.Format)
	logger.Info().
		Str("app", cfg.App.Name).
		Str("version", cfg.App.Version).
		Str("env", cfg.App.Env).
		Msg("starting application")

	db, err := database.Connect(cfg.DB)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()
	logger.Info().Str("host", cfg.DB.Host).Msg("database connected")

	// Run migrations automatically on startup
	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.DB.User, cfg.DB.Password, cfg.DB.Host, cfg.DB.Port, cfg.DB.Name, cfg.DB.SSLMode)
	if err := database.RunMigrations(dbURL, "migrations"); err != nil {
		logger.Fatal().Err(err).Msg("database migrations failed")
	}
	logger.Info().Msg("database migrations applied")

	ctx := context.Background()
	jwtValidator, err := auth.NewValidator(ctx, cfg.Auth0)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialise JWT validator (check AUTH0_DOMAIN)")
	}
	logger.Info().Str("domain", cfg.Auth0.Domain).Msg("Auth0 JWT validator ready")

	userRepo := userrepo.NewPostgresUserRepository(db.DB)
	chatRepo := chatrepo.NewPostgresChatRepository(db.DB)
	quotaRepo := chatrepo.NewPostgresQuotaRepository(db.DB)
	subRepo := subrepo.NewPostgresSubscriptionRepository(db.DB)
	subQuotaRepo := subrepo.NewPostgresSubscriptionQuotaRepository(db.DB)

	chatService := chatdomain.NewChatService(
		db.DB, chatRepo, quotaRepo, subQuotaRepo,
		cfg.AI.LatencyMinMs, cfg.AI.LatencyMaxMs,
	)
	subService := subdomain.NewSubscriptionService(subRepo)

	chatController := chatctrl.NewChatController(chatService)
	subController := subctrl.NewSubscriptionController(subService)
	adminController := adminctrl.NewAdminController(db.DB)
	healthController := adminctrl.NewHealthController(db.DB, cfg.App.Version)
	userRoleController := authctrl.NewGoogleAdminController(userRepo)

	nonceCache := middleware.NewNonceCache(time.Duration(cfg.Security.AntiReplayWindowSec*2) * time.Second)

	r := chi.NewRouter()

	r.Use(chimiddleware.Recoverer) // catch panics before they crash the server
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger(logger))
	r.Use(middleware.SecureHeaders)
	r.Use(middleware.RequestSizeLimit(cfg.Security.MaxRequestBodyBytes))
	r.Use(middleware.Timeout(time.Duration(cfg.Security.RequestTimeoutSec) * time.Second))

	// CORS — must be applied before any route handling
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.Security.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Nonce", "X-Request-Timestamp"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Global IP-based rate limiting (outermost defence)
	r.Use(middleware.RateLimitByIP(cfg.RateLimit.IPRPM))

	r.Get("/health", healthController.Health)

	r.With(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'")
			next.ServeHTTP(w, r)
		})
	}).Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.RequireJSON)

		r.Use(jwtValidator.Middleware)

		r.Use(middleware.AntiReplay(nonceCache, cfg.Security.AntiReplayWindowSec))

		r.Use(middleware.UserSync(userRepo))

		r.Use(middleware.RateLimitByUser(cfg.RateLimit.UserRPM))

		r.Route("/chat/messages", func(r chi.Router) {
			r.Use(middleware.RateLimitByUser(cfg.RateLimit.ChatRPM))
			chatController.Routes(r)
		})

		r.Route("/subscriptions", func(r chi.Router) {
			r.Use(middleware.RateLimitByUser(cfg.RateLimit.SubscriptionRPM))
			subController.Routes(r)
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(auth.RequireRole(auth.RoleAdmin))
			adminController.Routes(r)
			userRoleController.Routes(r)
		})
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.App.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: time.Duration(cfg.Security.RequestTimeoutSec+5) * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start the server in a goroutine so the main goroutine can block on signals.
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info().
			Str("addr", srv.Addr).
			Msg("HTTP server listening")
		serverErrors <- srv.ListenAndServe()
	}()

	// Background renewal job: configurable interval (default 1 hour).
	renewalInterval := time.Duration(cfg.App.RenewalIntervalMinutes) * time.Minute
	renewalCtx := logger.WithContext(context.Background())
	go func() {
		ticker := time.NewTicker(renewalInterval)
		defer ticker.Stop()
		for range ticker.C {
			if err := subService.ProcessRenewals(renewalCtx); err != nil {
				log.Error().Err(err).Msg("renewal job: error processing renewals")
			}
		}
	}()

	// Block until OS signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Fatal().Err(err).Msg("server error")

	case sig := <-quit:
		logger.Info().Str("signal", sig.String()).Msg("shutdown signal received")

		// Give in-flight requests up to 30 seconds to complete
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("graceful shutdown failed; forcing close")
			_ = srv.Close()
		}
		logger.Info().Msg("server stopped cleanly")
	}
}

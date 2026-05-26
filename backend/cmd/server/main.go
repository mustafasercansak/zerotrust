package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/admin"
	"github.com/zerotrust/backend/internal/audit"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/serviceaccount"
	"github.com/zerotrust/backend/internal/session"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
	authmw "github.com/zerotrust/backend/pkg/middleware"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := loadConfig()

	if err := database.RunMigrations(cfg.DatabaseURL, cfg.MigrationsPath); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")

	db, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
	})
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Error("redis connection failed", "error", err)
		os.Exit(1)
	}

	ks, err := auth.LoadOrGenerateKeyStore(cfg.JWTPrivateKeyFile, cfg.JWTSecondaryKeyFile)
	if err != nil {
		slog.Error("JWT key load failed", "error", err)
		os.Exit(1)
	}
	slog.Info("all connections and keys ready")

	userRepo := user.NewRepository(db)
	userSvc := user.NewService(userRepo)

	if cfg.InitialAdminEmail != "" && cfg.InitialAdminPasswordHash != "" {
		if err := userSvc.SeedAdmin(context.Background(), cfg.InitialAdminEmail, cfg.InitialAdminPasswordHash); err != nil {
			slog.Error("admin seed failed", "error", err)
			os.Exit(1)
		}
		slog.Info("admin seed complete", "email", cfg.InitialAdminEmail)
	}

	saRepo := serviceaccount.NewRepository(db)
	saSvc := serviceaccount.NewService(saRepo)
	saHub := serviceaccount.NewEventHub()
	go saHub.ListenForChanges(context.Background(), cfg.DatabaseURL)
	saHandler := serviceaccount.NewHandler(saSvc, saHub, ks)

	sessionRepo := session.NewRepository(db)
	sessionHandler := session.NewHandler(sessionRepo)
	go runSessionCleanup(sessionRepo)

	authSvc := auth.NewService(userSvc, sessionRepo, &saStoreAdapter{saSvc}, rdb, ks)
	auditRepo := audit.NewRepository(db)
	authHandler := auth.NewHandler(authSvc, userSvc, auditRepo, cfg.CookiesSecure)
	adminHandler := admin.NewHandler(userSvc)

	loginRL := authmw.NewRateLimiter(rdb, "login", 10, time.Minute)
	tokenRL := authmw.NewRateLimiter(rdb, "token", 30, time.Minute)
	globalRL := authmw.NewRateLimiter(rdb, "global", 300, time.Minute)

	r := chi.NewRouter()

	r.Use(authmw.SecurityHeaders(cfg.TLSEnabled))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"X-RateLimit-Limit", "X-RateLimit-Remaining"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(globalRL.Middleware())
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"zerotrust"}`)
	})

	// JWKS endpoint — allows external services to fetch public keys for local JWT validation
	r.Get("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		json.NewEncoder(w).Encode(ks.PublicJWKS())
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authmw.CSRF())

		r.With(loginRL.Middleware()).Post("/auth/login", authHandler.Login)
		r.With(tokenRL.Middleware()).Post("/auth/token", authHandler.Token) // client_credentials
		r.Post("/auth/refresh", authHandler.Refresh)
		r.Post("/auth/logout", authHandler.Logout)

		// SSE stream — auth handled inside handler via ?token= (EventSource can't send headers)
		r.Get("/admin/service-accounts/events", saHandler.Events)

		// Protected routes — ES256 + jti blocklist + audit log
		r.Group(func(r chi.Router) {
			r.Use(authmw.Authenticate(ks, authSvc))
			r.Use(authmw.AuditLog(auditRepo))

			r.Get("/me", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				roles := claims.Roles
				if roles == nil {
					roles = []string{}
				}
				perms := claims.Permissions
				if perms == nil {
					perms = []string{}
				}
				w.Header().Set("Content-Type", "application/json")
				rolesJSON, _ := json.Marshal(roles)
				permsJSON, _ := json.Marshal(perms)
				fmt.Fprintf(w, `{"user_id":%q,"email":%q,"locale":%q,"roles":%s,"permissions":%s}`,
					claims.UserID, claims.Email, claims.Locale, rolesJSON, permsJSON)
			})

			// User management — fine-grained permissions
			r.With(authmw.RequirePermission("users", "read")).Get("/admin/users", adminHandler.ListUsers)
			r.With(authmw.RequirePermission("users", "write")).Post("/admin/users", adminHandler.CreateUser)
			r.With(authmw.RequirePermission("users", "write")).Patch("/admin/users/{id}/roles", adminHandler.UpdateRoles)

			// Session management — any authenticated user manages their own sessions
			r.Get("/sessions", sessionHandler.List)
			r.Delete("/sessions/{id}", sessionHandler.Revoke)

			// Service account management
			r.With(authmw.RequirePermission("service_accounts", "read")).Get("/admin/service-accounts", saHandler.List)
			r.With(authmw.RequirePermission("service_accounts", "write")).Post("/admin/service-accounts", saHandler.Create)
			r.With(authmw.RequirePermission("service_accounts", "write")).Patch("/admin/service-accounts/{id}/status", saHandler.SetStatus)
			r.With(authmw.RequirePermission("service_accounts", "delete")).Delete("/admin/service-accounts/{id}", saHandler.Revoke)
		})
	})

	srv := &http.Server{
		Addr:         cfg.ServerAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("server starting", "addr", cfg.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

type config struct {
	ServerAddr               string
	DatabaseURL              string
	RedisAddr                string
	RedisPassword            string
	MigrationsPath           string
	JWTPrivateKeyFile        string
	JWTSecondaryKeyFile      string
	TLSEnabled               bool
	CookiesSecure            bool
	CORSOrigins              []string
	InitialAdminEmail        string
	InitialAdminPasswordHash string
}

func loadConfig() config {
	tlsEnabled := getEnv("TLS_ENABLED", "false") == "true"
	cookiesSecure := getEnv("COOKIES_SECURE", "false") == "true"
	origins := strings.Split(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"), ",")

	return config{
		ServerAddr:               getEnv("SERVER_ADDR", ":8080"),
		DatabaseURL:              getEnv("DATABASE_URL", "postgres://zerotrust:zerotrust_secret@localhost:5432/zerotrust_db?sslmode=disable"),
		RedisAddr:                getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:            getEnv("REDIS_PASSWORD", "zerotrust_secret"),
		MigrationsPath:           getEnv("MIGRATIONS_PATH", "migrations"),
		JWTPrivateKeyFile:        getEnv("JWT_PRIVATE_KEY_FILE", ""),
		JWTSecondaryKeyFile:      getEnv("JWT_SECONDARY_KEY_FILE", ""),
		TLSEnabled:               tlsEnabled,
		CookiesSecure:            cookiesSecure,
		CORSOrigins:              origins,
		InitialAdminEmail:        getEnv("INITIAL_ADMIN_EMAIL", ""),
		InitialAdminPasswordHash: getEnv("INITIAL_ADMIN_PASSWORD_HASH", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// runSessionCleanup deletes expired and revoked sessions every hour.
func runSessionCleanup(repo *session.Repository) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		n, err := repo.DeleteExpired(context.Background())
		if err != nil {
			slog.Error("session cleanup failed", "error", err)
		} else if n > 0 {
			slog.Info("session cleanup", "deleted", n)
		}
	}
}

// saStoreAdapter bridges *serviceaccount.Service to auth.ServiceAccountStore.
type saStoreAdapter struct{ svc *serviceaccount.Service }

func (a *saStoreAdapter) FindByClientID(ctx context.Context, clientID string) (*auth.ServiceAccountRecord, error) {
	sa, err := a.svc.FindByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}
	return &auth.ServiceAccountRecord{
		Name:             sa.Name,
		ClientSecretHash: sa.ClientSecretHash,
		Scopes:           sa.Scopes,
		IsActive:         sa.IsActive,
		ExpiresAt:        sa.ExpiresAt,
	}, nil
}

func (a *saStoreAdapter) CheckSecret(hash, secret string) bool {
	return a.svc.CheckSecret(hash, secret)
}

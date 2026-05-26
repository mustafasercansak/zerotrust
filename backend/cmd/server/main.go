package main

import (
	"context"
	"encoding/hex"
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
	"github.com/zerotrust/backend/internal/mfa"
	"github.com/zerotrust/backend/internal/passwdreset"
	"github.com/zerotrust/backend/internal/serviceaccount"
	"github.com/zerotrust/backend/internal/session"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
	"github.com/zerotrust/backend/pkg/mailer"
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
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()
	if err := db.Ping(pingCtx); err != nil {
		slog.Error("database ping failed", "error", err)
		os.Exit(1)
	}

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

	sessionRepo := session.NewRepository(db)
	sessionHandler := session.NewHandler(sessionRepo)
	go runSessionCleanup(sessionRepo)

	auditRepo := audit.NewRepository(db)
	auditHandler := audit.NewHandler(auditRepo)

	// MFA
	var mfaSvc *mfa.Service
	var mfaHandler *mfa.Handler
	if len(cfg.MFAEncryptionKey) == 32 {
		mfaRepo := mfa.NewRepository(db)
		mfaSvc = mfa.NewService(mfaRepo, cfg.MFAEncryptionKey, rdb)
		mfaHandler = mfa.NewHandler(mfaSvc)
		slog.Info("MFA enabled")
	} else {
		slog.Warn("MFA_ENCRYPTION_KEY not set or invalid — MFA disabled")
	}

	// Password reset mailer
	var ml mailer.Mailer = mailer.LogMailer{}
	if cfg.SMTPHost != "" {
		ml = mailer.NewSMTPMailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom, cfg.SMTPUser, cfg.SMTPPassword)
		slog.Info("SMTP mailer configured", "host", cfg.SMTPHost)
	} else {
		slog.Warn("SMTP_HOST not set — password reset emails will be logged only")
	}
	prRepo := passwdreset.NewRepository(db)
	prSvc := passwdreset.NewService(prRepo, userSvc, ml)

	var mfaChecker auth.MFAChecker
	if mfaSvc != nil {
		mfaChecker = mfaSvc
	}
	authSvc := auth.NewService(userSvc, sessionRepo, &saStoreAdapter{saSvc}, rdb, ks, mfaChecker)
	authHandler := auth.NewHandler(authSvc, userSvc, auditRepo, cfg.CookiesSecure, cfg.RegistrationEnabled, prSvc, cfg.PublicAppURL)
	adminHandler := admin.NewHandler(userSvc)
	saHandler := serviceaccount.NewHandler(saSvc, saHub, ks, authSvc)

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
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(globalRL.Middleware())
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"zerotrust"}`)
	})

	r.Get("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		json.NewEncoder(w).Encode(ks.PublicJWKS())
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authmw.CSRF())

		r.With(loginRL.Middleware()).Post("/auth/login", authHandler.Login)
		r.With(loginRL.Middleware()).Post("/auth/mfa/challenge", authHandler.MFAChallenge)
		r.With(tokenRL.Middleware()).Post("/auth/token", authHandler.Token)
		r.Post("/auth/refresh", authHandler.Refresh)
		r.Post("/auth/logout", authHandler.Logout)
		r.Post("/auth/register", authHandler.Register)
		r.With(loginRL.Middleware()).Post("/auth/forgot-password", authHandler.ForgotPassword)
		r.Post("/auth/reset-password", authHandler.ResetPassword)

		// SSE stream — auth handled inside handler via cookie (EventSource sends cookies automatically)
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

			// Session management — any authenticated user manages their own sessions
			r.Get("/sessions", sessionHandler.List)
			r.Delete("/sessions/{id}", sessionHandler.Revoke)

			// MFA management — any authenticated user
			if mfaHandler != nil {
				r.Get("/mfa/status", mfaHandler.Status)
				r.Post("/mfa/setup", mfaHandler.Setup)
				r.Post("/mfa/verify", mfaHandler.Verify)
				r.Post("/mfa/disable", mfaHandler.Disable)
			}

			// User management
			r.With(authmw.RequirePermission("users", "read")).Get("/admin/users", adminHandler.ListUsers)
			r.With(authmw.RequirePermission("users", "write")).Post("/admin/users", adminHandler.CreateUser)
			r.With(authmw.RequirePermission("users", "write")).Patch("/admin/users/{id}/roles", adminHandler.UpdateRoles)

			// Audit log
			r.With(authmw.RequirePermission("audit", "read")).Get("/admin/audit", auditHandler.List)

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
	RegistrationEnabled      bool
	CORSOrigins              []string
	InitialAdminEmail        string
	InitialAdminPasswordHash string
	MFAEncryptionKey         []byte
	SMTPHost                 string
	SMTPPort                 string
	SMTPFrom                 string
	SMTPUser                 string
	SMTPPassword             string
	PublicAppURL             string
}

func loadConfig() config {
	tlsEnabled := getEnv("TLS_ENABLED", "false") == "true"
	cookiesSecure := getEnv("COOKIES_SECURE", "false") == "true"
	registrationEnabled := getEnv("REGISTRATION_ENABLED", "false") == "true"
	origins := strings.Split(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"), ",")

	var mfaKey []byte
	if keyHex := getEnv("MFA_ENCRYPTION_KEY", ""); keyHex != "" {
		if b, err := hex.DecodeString(keyHex); err == nil && len(b) == 32 {
			mfaKey = b
		} else {
			slog.Warn("MFA_ENCRYPTION_KEY is set but invalid (must be 64 hex chars / 32 bytes)")
		}
	}

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
		RegistrationEnabled:      registrationEnabled,
		CORSOrigins:              origins,
		InitialAdminEmail:        getEnv("INITIAL_ADMIN_EMAIL", ""),
		InitialAdminPasswordHash: getEnv("INITIAL_ADMIN_PASSWORD_HASH", ""),
		MFAEncryptionKey:         mfaKey,
		SMTPHost:                 getEnv("SMTP_HOST", ""),
		SMTPPort:                 getEnv("SMTP_PORT", "587"),
		SMTPFrom:                 getEnv("SMTP_FROM", "noreply@localhost"),
		SMTPUser:                 getEnv("SMTP_USER", ""),
		SMTPPassword:             getEnv("SMTP_PASSWORD", ""),
		PublicAppURL:             getEnv("PUBLIC_APP_URL", "http://localhost:3000"),
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

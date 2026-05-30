package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
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
	"github.com/zerotrust/backend/internal/settings"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
	"github.com/zerotrust/backend/pkg/geoip"
	"github.com/zerotrust/backend/pkg/mailer"
	authmw "github.com/zerotrust/backend/pkg/middleware"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := loadConfig()
	if err != nil {
		slog.Error("configuration invalid", "error", err)
		os.Exit(1)
	}
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()
	rootCtx, cancelRoot := context.WithCancel(signalCtx)
	defer cancelRoot()

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

	sessionHub := session.NewEventHub()
	sessionRepo := session.NewRepository(db, sessionHub)

	var background sync.WaitGroup
	background.Add(1)
	go func() {
		defer background.Done()
		slog.Info("service account listener started")
		saHub.ListenForChanges(rootCtx, cfg.DatabaseURL)
		slog.Info("service account listener stopped")
	}()
	background.Add(1)
	go func() {
		defer background.Done()
		runSessionCleanup(rootCtx, sessionRepo)
	}()

	auditRepo := audit.NewRepository(db)
	auditHandler := audit.NewHandler(auditRepo)

	// MFA
	var mfaSvc *mfa.Service
	var mfaHandler *mfa.Handler
	const stepUpMFAWindow = 10 * time.Minute
	if cfg.MFAEnabled {
		mfaRepo := mfa.NewRepository(db)
		mfaSvc = mfa.NewService(mfaRepo, cfg.MFAEncryptionKey, rdb)
		mfaHandler = mfa.NewHandler(mfaSvc, rdb, stepUpMFAWindow)
		slog.Info("MFA enabled")
	} else {
		slog.Info("MFA disabled by configuration")
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

	// Resilient Mailer Wrapper
	resilientMailer := mailer.NewResilientMailer(ml, 1000, func(ctx context.Context, email, alertType, ip, details string, sendErr error) {
		var userID *string
		if u, err := userSvc.FindByEmail(ctx, email); err == nil {
			userID = &u.ID
		}
		meta := map[string]any{
			"email":      email,
			"alert_type": alertType,
			"error":      sendErr.Error(),
			"details":    details,
			"outcome":    "failure",
		}
		_ = auditRepo.Log(ctx, audit.Entry{
			UserID:    userID,
			Action:    "auth.security_alert.delivery_failure",
			Resource:  "auth",
			IPAddress: ip,
			Metadata:  meta,
		})
	})
	resilientMailer.Start(2)
	background.Add(1)
	go func() {
		defer background.Done()
		<-rootCtx.Done()
		resilientMailer.Stop()
	}()

	settingsRepo := settings.NewRepository(db)
	settingsCache := settings.NewCache(settingsRepo)
	settingsHandler := settings.NewHandler(settingsRepo)

	var mfaChecker auth.MFAChecker
	if mfaSvc != nil {
		mfaChecker = mfaSvc
	}
	stepUpMFA := authmw.RequireRecentMFA(mfaChecker, rdb, stepUpMFAWindow)
	authSvc := auth.NewService(userSvc, sessionRepo, &saStoreAdapter{saSvc}, rdb, ks, mfaChecker, settingsCache)
	geoipSvc := geoip.NewService(cfg.GeoIPDBPath)
	authSvc.ConfigureSecurityAnomalies(geoipSvc, resilientMailer)
	authHandler := auth.NewHandler(authSvc, userSvc, auditRepo, cfg.CookiesSecure, cfg.RegistrationEnabled, prSvc, cfg.PublicAppURL, settingsCache)
	sessionHandler := session.NewHandler(sessionRepo, sessionHub)
	adminHandler := admin.NewHandler(userSvc, sessionRepo)
	saHandler := serviceaccount.NewHandler(saSvc, saHub, ks, authSvc)

	loginRL := authmw.NewRateLimiter(rdb, "login", 10, time.Minute)
	tokenRL := authmw.NewRateLimiter(rdb, "token", 30, time.Minute)
	globalRL := authmw.NewRateLimiter(rdb, "global", 300, time.Minute)
	protectedRL := authmw.NewRateLimiter(rdb, "protected", 100, time.Minute)
	trustedCIDRs := authmw.ParseCIDRs(cfg.TrustedProxies)

	r := chi.NewRouter()

	r.Use(authmw.SecurityHeaders(cfg.TLSEnabled))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Retry-After", "X-RateLimit-Limit", "X-RateLimit-Remaining"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(chimiddleware.RequestID)
	r.Use(authmw.TrustedClientIP(trustedCIDRs))
	r.Use(skipPaths(globalRL.Middleware(), "/health", "/metrics"))
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","service":"zerotrust"}`)
	})

	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		writeMetrics(w)
	})

	r.Get("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		json.NewEncoder(w).Encode(ks.PublicJWKS())
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authmw.AuditCSRFFailures(auditRepo))
		r.Use(authmw.CSRF())

		publicAudit := authmw.AuditLog(auditRepo)
		r.With(publicAudit, loginRL.Middleware()).Post("/auth/login", authHandler.Login)
		r.With(publicAudit, loginRL.Middleware()).Post("/auth/mfa/challenge", authHandler.MFAChallenge)
		r.With(publicAudit, tokenRL.Middleware()).Post("/auth/token", authHandler.Token)
		r.With(publicAudit).Post("/auth/refresh", authHandler.Refresh)
		r.With(publicAudit).Post("/auth/logout", authHandler.Logout)
		r.With(publicAudit).Post("/auth/register", authHandler.Register)
		r.With(publicAudit, loginRL.Middleware()).Post("/auth/forgot-password", authHandler.ForgotPassword)
		r.With(publicAudit).Post("/auth/reset-password", authHandler.ResetPassword)

		// SSE stream — auth handled inside handler via cookie (EventSource sends cookies automatically)
		r.With(publicAudit).Get("/admin/service-accounts/events", saHandler.Events)

		// Protected routes — ES256 + jti blocklist + audit log
		r.Group(func(r chi.Router) {
			r.Use(authmw.AuditAuthFailures(auditRepo))
			r.Use(authmw.Authenticate(ks, authSvc))
			r.Use(protectedRL.Middleware())
			r.Use(authmw.AuditLog(auditRepo))

			r.Get("/me", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				profile, err := userRepo.FindByID(r.Context(), claims.UserID)
				if err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}
				roles := profile.Roles
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
				fmt.Fprintf(w, `{"user_id":%q,"email":%q,"first_name":%q,"last_name":%q,"has_avatar":%t,"locale":%q,"roles":%s,"permissions":%s}`,
					profile.ID, profile.Email, profile.FirstName, profile.LastName, profile.HasAvatar, profile.Locale, rolesJSON, permsJSON)
			})

			r.Patch("/me/profile", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				var req struct {
					FirstName string `json:"first_name"`
					LastName  string `json:"last_name"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
					return
				}
				profile, err := userSvc.UpdateProfile(r.Context(), claims.UserID, req.FirstName, req.LastName)
				if err != nil {
					if errors.Is(err, user.ErrInvalidProfile) {
						http.Error(w, `{"error":"invalid_profile"}`, http.StatusBadRequest)
						return
					}
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}
				roles := profile.Roles
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
				fmt.Fprintf(w, `{"user_id":%q,"email":%q,"first_name":%q,"last_name":%q,"has_avatar":%t,"locale":%q,"roles":%s,"permissions":%s}`,
					profile.ID, profile.Email, profile.FirstName, profile.LastName, profile.HasAvatar, profile.Locale, rolesJSON, permsJSON)
			})

			r.Patch("/me/locale", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				var req struct {
					Locale string `json:"locale"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
					return
				}
				allowed := map[string]bool{"tr": true, "en": true}
				if !allowed[req.Locale] {
					http.Error(w, `{"error":"invalid_locale"}`, http.StatusBadRequest)
					return
				}
				if err := userRepo.UpdateLocale(r.Context(), claims.UserID, req.Locale); err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})

			r.Post("/me/avatar", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
				if err := r.ParseMultipartForm(2 * 1024 * 1024); err != nil {
					http.Error(w, `{"error":"file_too_large"}`, http.StatusBadRequest)
					return
				}
				file, header, err := r.FormFile("avatar")
				if err != nil {
					http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
					return
				}
				defer file.Close()

				// Sniff the first 512 bytes of the file to detect its actual content type.
				buf := make([]byte, 512)
				n, err := file.Read(buf)
				if err != nil && err != io.EOF {
					http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
					return
				}
				detectedType := http.DetectContentType(buf[:n])
				if detectedType != "image/jpeg" && detectedType != "image/png" {
					http.Error(w, `{"error":"invalid_file_type"}`, http.StatusBadRequest)
					return
				}

				// Reset read pointer to the beginning of the file so io.Copy can write the whole content.
				if _, err := file.Seek(0, io.SeekStart); err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}

				uploadDir := "uploads/avatars"
				if err := os.MkdirAll(uploadDir, 0755); err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}

				filePath := filepath.Join(uploadDir, claims.UserID)
				out, err := os.Create(filePath)
				if err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}
				defer out.Close()

				if _, err := io.Copy(out, file); err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}

				profile, err := userSvc.UpdateAvatar(r.Context(), claims.UserID, claims.UserID, int(header.Size))
				if err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}

				roles := profile.Roles
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
				fmt.Fprintf(w, `{"user_id":%q,"email":%q,"first_name":%q,"last_name":%q,"has_avatar":%t,"locale":%q,"roles":%s,"permissions":%s}`,
					profile.ID, profile.Email, profile.FirstName, profile.LastName, profile.HasAvatar, profile.Locale, rolesJSON, permsJSON)
			})

			r.Get("/me/avatar", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				u, err := userSvc.FindByID(r.Context(), claims.UserID)
				if err != nil || !u.HasAvatar {
					http.NotFound(w, r)
					return
				}
				filePath := filepath.Join("uploads/avatars", claims.UserID)
				http.ServeFile(w, r, filePath)
			})

			r.Get("/users/{id}/avatar", func(w http.ResponseWriter, r *http.Request) {
				id := chi.URLParam(r, "id")
				u, err := userSvc.FindByID(r.Context(), id)
				if err != nil || !u.HasAvatar {
					http.NotFound(w, r)
					return
				}
				filePath := filepath.Join("uploads/avatars", id)
				http.ServeFile(w, r, filePath)
			})

			r.Delete("/me/avatar", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				filePath := filepath.Join("uploads/avatars", claims.UserID)
				_ = os.Remove(filePath)

				profile, err := userSvc.UpdateAvatar(r.Context(), claims.UserID, "", 0)
				if err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}

				roles := profile.Roles
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
				fmt.Fprintf(w, `{"user_id":%q,"email":%q,"first_name":%q,"last_name":%q,"has_avatar":%t,"locale":%q,"roles":%s,"permissions":%s}`,
					profile.ID, profile.Email, profile.FirstName, profile.LastName, profile.HasAvatar, profile.Locale, rolesJSON, permsJSON)
			})

			// Session management — any authenticated user manages their own sessions
			r.Get("/sessions", sessionHandler.List)
			r.Get("/sessions/events", sessionHandler.Events)
			r.Delete("/sessions", sessionHandler.RevokeOthers)
			r.Delete("/sessions/{id}", sessionHandler.Revoke)

			// MFA management — any authenticated user
			if mfaHandler != nil {
				r.Get("/mfa/status", mfaHandler.Status)
				r.Post("/mfa/setup", mfaHandler.Setup)
				r.Post("/mfa/verify", mfaHandler.Verify)
				r.Post("/mfa/disable", mfaHandler.Disable)
				r.Post("/mfa/step-up", mfaHandler.StepUp)
			} else {
				r.Get("/mfa/status", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"enabled":false,"supported":false}`))
				})
			}

			// User management
			r.With(authmw.RequirePermission("users", "read")).Get("/admin/users", adminHandler.ListUsers)
			r.With(authmw.RequirePermission("users", "create")).Post("/admin/users", adminHandler.CreateUser)
			r.With(authmw.RequirePermission("users", "update"), stepUpMFA).Patch("/admin/users/{id}/roles", adminHandler.UpdateRoles)
			r.With(authmw.RequirePermission("users", "update"), stepUpMFA).Patch("/admin/users/{id}/status", adminHandler.SetStatus)
			r.With(authmw.RequirePermission("users", "read")).Get("/admin/users/{id}/sessions", adminHandler.ListUserSessions)
			r.With(authmw.RequirePermission("users", "update"), stepUpMFA).Delete("/admin/users/{id}/sessions", adminHandler.RevokeAllUserSessions)
			r.With(authmw.RequirePermission("users", "update"), stepUpMFA).Delete("/admin/users/{id}/sessions/{sessionId}", adminHandler.RevokeUserSession)

			// System settings — admin role only
			r.With(authmw.RequireRole("admin")).Get("/admin/settings", settingsHandler.List)
			r.With(authmw.RequireRole("admin"), stepUpMFA).Patch("/admin/settings", settingsHandler.Update)

			// Audit log
			r.With(authmw.RequirePermission("audit", "read")).Get("/admin/audit", auditHandler.List)
			r.With(authmw.RequirePermission("audit", "read")).Get("/admin/audit/trends", auditHandler.Trends)

			// Service account management
			r.With(authmw.RequirePermission("service_accounts", "read")).Get("/admin/service-accounts", saHandler.List)
			r.With(authmw.RequirePermission("service_accounts", "create"), stepUpMFA).Post("/admin/service-accounts", saHandler.Create)
			r.With(authmw.RequirePermission("service_accounts", "update"), stepUpMFA).Patch("/admin/service-accounts/{id}", saHandler.Update)
			r.With(authmw.RequirePermission("service_accounts", "update"), stepUpMFA).Patch("/admin/service-accounts/{id}/status", saHandler.SetStatus)
			r.With(authmw.RequirePermission("service_accounts", "update"), stepUpMFA).Post("/admin/service-accounts/{id}/rotate", saHandler.RotateSecret)
			r.With(authmw.RequirePermission("service_accounts", "delete"), stepUpMFA).Delete("/admin/service-accounts/{id}", saHandler.Revoke)
		})
	})

	srv := &http.Server{
		Addr:         cfg.ServerAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", cfg.ServerAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	serverFailed := false
	select {
	case <-rootCtx.Done():
		slog.Info("shutdown signal received")
	case err := <-serverErr:
		serverFailed = true
		slog.Error("server error", "error", err)
		cancelRoot()
		stopSignals()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown failed", "error", err)
	} else {
		slog.Info("server stopped")
	}

	workerCtx, workerCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer workerCancel()
	if waitForBackground(workerCtx, &background) {
		slog.Info("background workers stopped")
	} else {
		slog.Error("background workers did not stop before timeout")
	}
	if serverFailed {
		os.Exit(1)
	}
}

func writeMetrics(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP zerotrust_audit_write_failures_total Total audit log write failures.\n")
	fmt.Fprintf(w, "# TYPE zerotrust_audit_write_failures_total counter\n")
	fmt.Fprintf(w, "zerotrust_audit_write_failures_total %d\n", audit.WriteFailures())
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
	MFAEnabled               bool
	CORSOrigins              []string
	TrustedProxies           string
	InitialAdminEmail        string
	InitialAdminPasswordHash string
	MFAEncryptionKey         []byte
	SMTPHost                 string
	SMTPPort                 string
	SMTPFrom                 string
	SMTPUser                 string
	SMTPPassword             string
	PublicAppURL             string
	GeoIPDBPath              string
}

func loadConfig() (config, error) {
	tlsEnabled := getEnv("TLS_ENABLED", "false") == "true"
	cookiesSecure := getEnv("COOKIES_SECURE", "false") == "true"
	registrationEnabled := getEnv("REGISTRATION_ENABLED", "false") == "true"
	mfaEnabled, err := boolEnv("MFA_ENABLED", false)
	if err != nil {
		return config{}, err
	}
	origins := strings.Split(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"), ",")

	keyHex := getEnv("MFA_ENCRYPTION_KEY", "")
	if !mfaEnabled {
		if keyHex != "" {
			slog.Warn("MFA_ENCRYPTION_KEY is set but ignored because MFA_ENABLED is false")
		}
		keyHex = ""
	} else if keyHex == "" {
		return config{}, fmt.Errorf("MFA_ENABLED=true requires MFA_ENCRYPTION_KEY")
	}

	var mfaKey []byte
	if keyHex != "" {
		b, err := hex.DecodeString(keyHex)
		if err != nil || len(b) != 32 {
			return config{}, fmt.Errorf("MFA_ENABLED=true requires MFA_ENCRYPTION_KEY to be 64 hex chars / 32 bytes")
		}
		mfaKey = b
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
		MFAEnabled:               mfaEnabled,
		CORSOrigins:              origins,
		TrustedProxies:           getEnv("TRUSTED_PROXIES", ""),
		InitialAdminEmail:        getEnv("INITIAL_ADMIN_EMAIL", ""),
		InitialAdminPasswordHash: getEnv("INITIAL_ADMIN_PASSWORD_HASH", ""),
		MFAEncryptionKey:         mfaKey,
		SMTPHost:                 getEnv("SMTP_HOST", ""),
		SMTPPort:                 getEnv("SMTP_PORT", "587"),
		SMTPFrom:                 getEnv("SMTP_FROM", "noreply@localhost"),
		SMTPUser:                 getEnv("SMTP_USER", ""),
		SMTPPassword:             getEnv("SMTP_PASSWORD", ""),
		PublicAppURL:             getEnv("PUBLIC_APP_URL", "http://localhost:3000"),
		GeoIPDBPath:              getEnv("GEOIP_DB_PATH", "./GeoLite2-City.mmdb"),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func boolEnv(key string, fallback bool) (bool, error) {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback, nil
	}
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", key)
	}
}

func skipPaths(mw func(http.Handler) http.Handler, paths ...string) func(http.Handler) http.Handler {
	skipped := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		skipped[path] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		wrapped := mw(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := skipped[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}
			wrapped.ServeHTTP(w, r)
		})
	}
}

// runSessionCleanup runs two periodic tasks:
//   - Every 30 s: revoke sessions whose access token was never refreshed (abandoned
//     logins, bots, closed tabs). Other connected clients see a real-time "change"
//     event and show a snackbar.
//   - Every hour: delete rows that are expired or already revoked.
type sessionCleaner interface {
	RevokeStaleInitialSessions(ctx context.Context) (int64, error)
	DeleteExpired(ctx context.Context) (int64, error)
}

func waitForBackground(ctx context.Context, wg *sync.WaitGroup) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

func runSessionCleanup(ctx context.Context, repo sessionCleaner) {
	runSessionCleanupLoop(ctx, repo, 30*time.Second, time.Hour)
}

func runSessionCleanupLoop(ctx context.Context, repo sessionCleaner, staleInterval, purgeInterval time.Duration) {
	staleTicker := time.NewTicker(staleInterval)
	purgeTicker := time.NewTicker(purgeInterval)
	defer staleTicker.Stop()
	defer purgeTicker.Stop()

	slog.Info("session cleanup worker started")
	defer slog.Info("session cleanup worker stopped")

	for {
		select {
		case <-ctx.Done():
			return
		case <-staleTicker.C:
			if ctx.Err() != nil {
				return
			}
			n, err := repo.RevokeStaleInitialSessions(ctx)
			if err != nil {
				slog.Error("stale session revocation failed", "error", err)
			} else if n > 0 {
				slog.Info("stale initial sessions revoked", "count", n)
			}
		case <-purgeTicker.C:
			if ctx.Err() != nil {
				return
			}
			n, err := repo.DeleteExpired(ctx)
			if err != nil {
				slog.Error("session cleanup failed", "error", err)
			} else if n > 0 {
				slog.Info("session cleanup", "deleted", n)
			}
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
		Name:                sa.Name,
		ClientSecretHash:    sa.ClientSecretHash,
		Scopes:              sa.Scopes,
		IsActive:            sa.IsActive,
		ExpiresAt:           sa.ExpiresAt,
		OldClientSecretHash: sa.OldClientSecretHash,
		OldSecretExpiresAt:  sa.OldSecretExpiresAt,
	}, nil
}

func (a *saStoreAdapter) CheckSecret(hash, secret string) bool {
	return a.svc.CheckSecret(hash, secret)
}

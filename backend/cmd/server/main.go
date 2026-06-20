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
	"strconv"
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
	"github.com/zerotrust/backend/internal/oidc"
	"github.com/zerotrust/backend/internal/passwdreset"
	"github.com/zerotrust/backend/internal/serviceaccount"
	"github.com/zerotrust/backend/internal/session"
	"github.com/zerotrust/backend/internal/settings"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/internal/webauthn"
	"github.com/zerotrust/backend/pkg/database"
	"github.com/zerotrust/backend/pkg/geoip"
	"github.com/zerotrust/backend/pkg/mailer"
	authmw "github.com/zerotrust/backend/pkg/middleware"
	"github.com/zerotrust/backend/pkg/secrets"
	"github.com/zerotrust/backend/pkg/validation"
	"golang.org/x/crypto/bcrypt"
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

	if err := run(signalCtx, cfg); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// run contains the full server lifecycle: initialise dependencies, register routes,
// start the HTTP server, and block until ctx is cancelled or the server fails.
// Returning an error causes main() to exit with code 1.
func run(ctx context.Context, cfg config) error {
	rootCtx, cancelRoot := context.WithCancel(ctx)
	defer cancelRoot()

	if err := database.RunMigrations(cfg.DatabaseURL, cfg.MigrationsPath); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}
	slog.Info("migrations applied")

	dbCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database parse config failed: %w", err)
	}
	if cfg.DatabaseMaxConns > 0 {
		dbCfg.MaxConns = int32(cfg.DatabaseMaxConns)
	}
	if cfg.DatabaseMinConns > 0 {
		dbCfg.MinConns = int32(cfg.DatabaseMinConns)
	}
	if cfg.DatabaseConnTimeout > 0 {
		dbCfg.ConnConfig.ConnectTimeout = cfg.DatabaseConnTimeout
	}

	db, err := pgxpool.NewWithConfig(context.Background(), dbCfg)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer db.Close()
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pingCancel()
	if err := db.Ping(pingCtx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	slog.Info("database connection pool initialized",
		"max_conns", db.Stat().MaxConns(),
		"total_conns", db.Stat().TotalConns(),
		"idle_conns", db.Stat().IdleConns(),
	)

	redisOpts := &redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
	}
	if cfg.RedisPoolSize > 0 {
		redisOpts.PoolSize = cfg.RedisPoolSize
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("redis connection failed: %w", err)
	}
	slog.Info("redis connection pool initialized",
		"pool_size", rdb.Options().PoolSize,
		"total_conns", rdb.PoolStats().TotalConns,
		"idle_conns", rdb.PoolStats().IdleConns,
	)

	ks, err := auth.LoadOrGenerateKeyStore(cfg.JWTPrivateKeyFile, cfg.JWTSecondaryKeyFile)
	if err != nil {
		return fmt.Errorf("JWT key load failed: %w", err)
	}
	slog.Info("all connections and keys ready")

	userRepo := user.NewRepository(db)
	// Initialize Secrets Client
	var secClient *secrets.Client
	if os.Getenv("BAO_ADDR") != "" || os.Getenv("VAULT_ADDR") != "" {
		var sErr error
		secClient, sErr = secrets.NewClient("db-encryption-key")
		if sErr != nil {
			return fmt.Errorf("failed to initialize secrets client: %w", sErr)
		}
		checkCtx, cancelCheck := context.WithTimeout(rootCtx, 5*time.Second)
		sErr = secClient.Check(checkCtx)
		cancelCheck()
		if sErr != nil {
			return fmt.Errorf("failed to validate secrets client: %w", sErr)
		}
		slog.Info("secrets client initialized for application-level encryption")
		userRepo.SetSecretsClient(secClient)
	} else {
		slog.Warn("BAO_ADDR/VAULT_ADDR not set — secrets client disabled (running without encryption)")
	}
	userSvc := user.NewService(userRepo)

	if cfg.InitialAdminEmail != "" && cfg.InitialAdminPasswordHash != "" {
		if err := userSvc.SeedAdmin(context.Background(), cfg.InitialAdminEmail, cfg.InitialAdminPasswordHash); err != nil {
			return fmt.Errorf("admin seed failed: %w", err)
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
	background.Add(1)
	go func() {
		defer background.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		slog.Info("connection pool metrics worker started")
		for {
			select {
			case <-rootCtx.Done():
				slog.Info("connection pool metrics worker stopped")
				return
			case <-ticker.C:
				dbStat := db.Stat()
				rdbStat := rdb.PoolStats()
				slog.Info("connection pool metrics",
					"db_max_conns", dbStat.MaxConns(),
					"db_total_conns", dbStat.TotalConns(),
					"db_idle_conns", dbStat.IdleConns(),
					"redis_total_conns", rdbStat.TotalConns,
					"redis_idle_conns", rdbStat.IdleConns,
				)
			}
		}
	}()

	auditRepo := audit.NewRepository(db)
	if secClient != nil {
		auditRepo.SetSecretsClient(secClient)
	}
	auditHandler := audit.NewHandler(auditRepo)

	// MFA
	var mfaRepo *mfa.Repository
	var mfaSvc *mfa.Service
	var mfaHandler *mfa.Handler
	const stepUpMFAWindow = 10 * time.Minute
	if cfg.MFAEnabled {
		mfaRepo = mfa.NewRepository(db)
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
	auditRepo.SetSettingsReader(settingsCache)
	settingsHandler := settings.NewHandler(settingsRepo)

	var mfaChecker auth.MFAChecker
	if mfaSvc != nil {
		mfaChecker = mfaSvc
	}
	stepUpMFA := authmw.RequireRecentMFA(mfaChecker, rdb, stepUpMFAWindow)
	authSvc := auth.NewService(userSvc, sessionRepo, &saStoreAdapter{saSvc}, rdb, ks, mfaChecker, settingsCache)
	geoipSvc := geoip.NewService(cfg.GeoIPDBPath)
	authSvc.ConfigureSecurityAnomalies(geoipSvc, resilientMailer)
	auditRepo.SetIPLocator(func(ip string) (string, string) {
		loc, err := geoipSvc.Lookup(ip)
		if err != nil || loc == nil {
			return "", ""
		}
		return loc.Country, loc.City
	})
	// Enable DPoP htu host binding when an external API origin is configured. (#36)
	auth.SetExpectedDPoPOrigin(cfg.DPoPExpectedOrigin)

	// WebAuthn / passkeys — phishing-resistant second factor. The browser origin
	// is the frontend, so the RP origins are the configured CORS origins.
	webauthnRepo := webauthn.NewRepository(db)
	webauthnSvc, err := webauthn.NewService(webauthnRepo, rdb, webauthn.Config{
		RPID:          cfg.WebAuthnRPID,
		RPDisplayName: cfg.WebAuthnRPDisplayName,
		RPOrigins:     cfg.CORSOrigins,
	}, settingsCache)
	if err != nil {
		return fmt.Errorf("webauthn init: %w", err)
	}
	authSvc.ConfigureWebAuthn(webauthnSvc)
	webauthnHandler := webauthn.NewHandler(webauthnSvc)
	webauthnHandler.ConfigureNotifier(resilientMailer)
	if mfaHandler != nil {
		mfaHandler.ConfigureNotifier(resilientMailer)
	}

	oidcRepo := oidc.NewClientRepository(db)
	oidcCodeStore := oidc.NewAuthCodeStore(rdb)
	oidcRefreshStore := oidc.NewRefreshTokenStore(rdb)
	oidcSvc := oidc.NewService(oidcRepo, oidcCodeStore, userSvc, ks, cfg.OIDCIssuerURL, oidcRefreshStore)
	oidcHandler := oidc.NewHandler(oidcSvc, oidcRepo, userSvc, authSvc, ks, cfg.OIDCIssuerURL, cfg.PublicAppURL, auditRepo, mfaSvc, rdb)

	authHandler := auth.NewHandler(authSvc, userSvc, auditRepo, cfg.CookiesSecure, cfg.RegistrationEnabled, prSvc, cfg.PublicAppURL, settingsCache)
	sessionHandler := session.NewHandler(sessionRepo, sessionHub)
	adminHandler := admin.NewHandler(userSvc, sessionRepo, webauthnRepo, mfaRepo)
	adminHandler.SetPostureProvider(userRepo)
	saHandler := serviceaccount.NewHandler(saSvc, saHub, ks, authSvc)

	loginRL := authmw.NewRateLimiter(rdb, "login", 10, time.Minute)
	tokenRL := authmw.NewRateLimiter(rdb, "token", 30, time.Minute)
	globalRL := authmw.NewRateLimiter(rdb, "global", 300, time.Minute)
	protectedRL := authmw.NewRateLimiter(rdb, "protected", 300, time.Minute)
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
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		type checks struct {
			Database string `json:"database"`
			Redis    string `json:"redis"`
		}
		type healthResponse struct {
			Status  string `json:"status"`
			Service string `json:"service"`
			Checks  checks `json:"checks"`
		}

		chk := checks{Database: "ok", Redis: "ok"}
		if err := db.Ping(ctx); err != nil {
			chk.Database = "error"
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			chk.Redis = "error"
		}

		overall := "ok"
		if chk.Database != "ok" || chk.Redis != "ok" {
			overall = "degraded"
		}

		status := http.StatusOK
		if overall == "degraded" {
			status = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(healthResponse{
			Status:  overall,
			Service: "zerotrust",
			Checks:  chk,
		})
	})

	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		writeMetrics(w)
	})

	r.Get("/.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		json.NewEncoder(w).Encode(ks.PublicJWKS())
	})

	r.Get("/.well-known/openid-configuration", oidcHandler.Discovery)
	r.With(tokenRL.Middleware()).Get("/oauth2/authorize", oidcHandler.Authorize)
	r.With(tokenRL.Middleware()).Post("/oauth2/token", oidcHandler.Token)
	r.With(protectedRL.Middleware()).Get("/oauth2/userinfo", oidcHandler.UserInfo)
	r.With(tokenRL.Middleware()).Post("/oauth2/revoke", oidcHandler.Revoke)
	r.With(tokenRL.Middleware()).Post("/oauth2/introspect", oidcHandler.Introspect)
	r.With(tokenRL.Middleware()).Get("/oauth2/end_session", oidcHandler.EndSession)
	r.With(tokenRL.Middleware()).Post("/oauth2/end_session", oidcHandler.EndSession)
	r.Get("/oauth2/clients/{client_id}", oidcHandler.GetPublicClient)

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(authmw.AuditCSRFFailures(auditRepo))
		r.Use(authmw.CSRF())

		publicAudit := authmw.AuditLog(auditRepo)
		r.With(publicAudit, loginRL.Middleware()).Post("/auth/login", authHandler.Login)
		r.With(publicAudit, loginRL.Middleware()).Post("/auth/mfa/challenge", authHandler.MFAChallenge)
		r.With(publicAudit, loginRL.Middleware()).Post("/auth/webauthn/login/begin", authHandler.WebAuthnLoginBegin)
		r.With(publicAudit, loginRL.Middleware()).Post("/auth/webauthn/login/finish", authHandler.WebAuthnLoginFinish)
		r.With(publicAudit, loginRL.Middleware()).Post("/auth/webauthn/passwordless/begin", authHandler.WebAuthnPasswordlessBegin)
		r.With(publicAudit, loginRL.Middleware()).Post("/auth/webauthn/passwordless/finish", authHandler.WebAuthnPasswordlessFinish)
		r.With(publicAudit, tokenRL.Middleware()).Post("/auth/token", authHandler.Token)
		r.With(publicAudit).Post("/auth/refresh", authHandler.Refresh)
		r.With(publicAudit).Post("/auth/logout", authHandler.Logout)
		r.With(publicAudit).Post("/auth/register", authHandler.Register)
		r.With(publicAudit, loginRL.Middleware()).Post("/auth/forgot-password", authHandler.ForgotPassword)
		r.With(publicAudit).Post("/auth/reset-password", authHandler.ResetPassword)
		r.With(protectedRL.Middleware()).Post("/oauth2/consent", oidcHandler.Consent)

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
				w.Header().Set("Content-Type", "application/json")
				// Use the JSON encoder rather than hand-formatting so
				// user-controlled fields are always correctly escaped. (#37)
				_ = json.NewEncoder(w).Encode(buildMeResponse(profile, claims.Permissions))
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
				fmt.Fprintf(w, `{"user_id":%q,"email":%q,"first_name":%q,"last_name":%q,"has_avatar":%t,"locale":%q,"notify_security_emails":%t,"roles":%s,"permissions":%s}`,
					profile.ID, profile.Email, profile.FirstName, profile.LastName, profile.HasAvatar, profile.Locale, profile.NotifySecurityEmails, rolesJSON, permsJSON)
			})

			r.Get("/session/policy", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				isAdmin := false
				for _, role := range claims.Roles {
					if role == "admin" {
						isAdmin = true
						break
					}
				}
				var idleTimeout int
				if isAdmin {
					idleTimeout = settingsCache.GetInt(r.Context(), "session_idle_timeout_seconds_admin", 180)
				} else {
					idleTimeout = settingsCache.GetInt(r.Context(), "session_idle_timeout_seconds", 300)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"idle_timeout_seconds":%d}`, idleTimeout)
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
				existing, err := userRepo.FindByID(r.Context(), claims.UserID)
				if err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}
				if err := userRepo.UpdateLocale(r.Context(), claims.UserID, req.Locale); err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}
				if existing.Locale != req.Locale {
					ip := r.RemoteAddr
					if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
						ip = strings.SplitN(xf, ",", 2)[0]
					}
					uid := claims.UserID
					_ = auditRepo.Log(r.Context(), audit.Entry{
						UserID:    &uid,
						Action:    "user.locale_changed",
						Resource:  "user",
						IPAddress: ip,
						Metadata: map[string]any{
							"from":    existing.Locale,
							"to":     req.Locale,
							"outcome": "success",
						},
					})
					if ml != nil && existing.NotifySecurityEmails {
						_ = ml.SendSecurityAlert(r.Context(), existing.Email,
							"locale_changed", ip, "Unknown",
							fmt.Sprintf("Your ZeroTrust interface language was changed from %q to %q. If this was not you, review your account immediately.", existing.Locale, req.Locale),
						)
					}
				}
				w.WriteHeader(http.StatusNoContent)
			})

			r.Patch("/me/password", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				var req struct {
					CurrentPassword string `json:"current_password"`
					NewPassword     string `json:"new_password"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
					return
				}
				if req.CurrentPassword == "" || req.NewPassword == "" {
					http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
					return
				}

				profile, err := userRepo.FindByID(r.Context(), claims.UserID)
				if err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}
				if !userSvc.CheckPassword(profile.PasswordHash, req.CurrentPassword) {
					http.Error(w, `{"error":"wrong_password"}`, http.StatusUnauthorized)
					return
				}

				complexity := settingsCache.GetString(r.Context(), "password_complexity", "low")
				if err := validation.PasswordWithComplexity(req.NewPassword, complexity); err != nil {
					http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
					return
				}

				newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
				if err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}
				if err := userSvc.UpdatePassword(r.Context(), claims.UserID, string(newHash)); err != nil {
					http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
					return
				}
				// Notify the user that their password was changed regardless of their
				// notification preference — this is a security-critical alert.
				if ml != nil {
					ip := r.RemoteAddr
					if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
						ip = strings.SplitN(xf, ",", 2)[0]
					}
					_ = ml.SendSecurityAlert(r.Context(), profile.Email,
						"password_changed", ip, "Unknown",
						"Your ZeroTrust password was just changed. If this was not you, revoke all sessions immediately from your settings page.",
					)
				}
				w.WriteHeader(http.StatusNoContent)
			})

			r.Patch("/me/notifications", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				var req struct {
					NotifySecurityEmails bool `json:"notify_security_emails"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
					return
				}
				if err := userRepo.UpdateNotifySecurityEmails(r.Context(), claims.UserID, req.NotifySecurityEmails); err != nil {
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
				fmt.Fprintf(w, `{"user_id":%q,"email":%q,"first_name":%q,"last_name":%q,"has_avatar":%t,"locale":%q,"notify_security_emails":%t,"roles":%s,"permissions":%s}`,
					profile.ID, profile.Email, profile.FirstName, profile.LastName, profile.HasAvatar, profile.Locale, profile.NotifySecurityEmails, rolesJSON, permsJSON)
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
				fmt.Fprintf(w, `{"user_id":%q,"email":%q,"first_name":%q,"last_name":%q,"has_avatar":%t,"locale":%q,"notify_security_emails":%t,"roles":%s,"permissions":%s}`,
					profile.ID, profile.Email, profile.FirstName, profile.LastName, profile.HasAvatar, profile.Locale, profile.NotifySecurityEmails, rolesJSON, permsJSON)
			})

			// Session management — any authenticated user manages their own sessions
			r.Get("/sessions", sessionHandler.List)
			r.Get("/sessions/events", sessionHandler.Events)
			r.Delete("/sessions", sessionHandler.RevokeOthers)
			r.Delete("/sessions/{id}", sessionHandler.Revoke)

			// Own audit log — user sees only their own entries
			r.Get("/me/audit", func(w http.ResponseWriter, r *http.Request) {
				claims := authmw.ClaimsFrom(r.Context())
				if claims == nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
					return
				}
				q := r.URL.Query()
				limit, offset := 25, 0
				if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
					limit = n
				}
				if n, err := strconv.Atoi(q.Get("offset")); err == nil && n >= 0 {
					offset = n
				}
				result, err := auditRepo.List(r.Context(), audit.ListParams{
					Limit:              limit,
					Offset:             offset,
					SortBy:             q.Get("sort_by"),
					SortDir:            q.Get("sort_dir"),
					UserID:             claims.UserID,
					SecurityEventsOnly: true,
				})
				if err != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": "internal_error"})
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"data": result.Entries, "total": result.Total})
			})

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

			// WebAuthn / passkey management — any authenticated user
			r.Post("/webauthn/register/begin", webauthnHandler.RegisterBegin)
			r.Post("/webauthn/register/finish", webauthnHandler.RegisterFinish)
			r.Get("/webauthn/credentials", webauthnHandler.List)
			r.Delete("/webauthn/credentials/{id}", webauthnHandler.Delete)

			// User management
			r.With(authmw.RequirePermission("users", "read")).Get("/admin/users", adminHandler.ListUsers)
			r.With(authmw.RequirePermission("users", "create")).Post("/admin/users", adminHandler.CreateUser)
			r.With(authmw.RequirePermission("users", "update"), stepUpMFA).Post("/admin/users/bulk-status", adminHandler.BulkSetStatus)
			r.With(authmw.RequirePermission("users", "update"), stepUpMFA).Patch("/admin/users/{id}/roles", adminHandler.UpdateRoles)
			r.With(authmw.RequirePermission("users", "update"), stepUpMFA).Patch("/admin/users/{id}/status", adminHandler.SetStatus)
			r.With(authmw.RequirePermission("users", "read")).Get("/admin/users/{id}/sessions", adminHandler.ListUserSessions)
			r.With(authmw.RequirePermission("users", "read")).Get("/admin/users/{id}/mfa", adminHandler.GetUserMfa)
			r.With(authmw.RequirePermission("users", "update"), stepUpMFA).Delete("/admin/users/{id}/sessions", adminHandler.RevokeAllUserSessions)
			r.With(authmw.RequirePermission("users", "update"), stepUpMFA).Delete("/admin/users/{id}/sessions/{sessionId}", adminHandler.RevokeUserSession)

			// Security posture summary — admin role only
			r.With(authmw.RequireRole("admin")).Get("/admin/security-posture", adminHandler.SecurityPosture)

			// System health — admin role only (includes pool stats)
			r.With(authmw.RequireRole("admin")).Get("/admin/health", func(w http.ResponseWriter, r *http.Request) {
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()

				dbStatus, redisStatus := "ok", "ok"
				if err := db.Ping(ctx); err != nil {
					dbStatus = "error"
				}
				if err := rdb.Ping(ctx).Err(); err != nil {
					redisStatus = "error"
				}

				overall := "ok"
				if dbStatus != "ok" || redisStatus != "ok" {
					overall = "degraded"
				}

				dbStat := db.Stat()
				rdbStat := rdb.PoolStats()

				type poolStats struct {
					Total int32 `json:"total"`
					Idle  int32 `json:"idle"`
					Max   int32 `json:"max"`
				}
				type svcHealth struct {
					Status string    `json:"status"`
					Pool   poolStats `json:"pool"`
				}
				type adminHealthResponse struct {
					Status   string    `json:"status"`
					Database svcHealth `json:"database"`
					Redis    svcHealth `json:"redis"`
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(adminHealthResponse{
					Status: overall,
					Database: svcHealth{
						Status: dbStatus,
						Pool: poolStats{
							Total: dbStat.TotalConns(),
							Idle:  dbStat.IdleConns(),
							Max:   dbStat.MaxConns(),
						},
					},
					Redis: svcHealth{
						Status: redisStatus,
						Pool: poolStats{
							Total: int32(rdbStat.TotalConns),
							Idle:  int32(rdbStat.IdleConns),
							Max:   int32(rdb.Options().PoolSize),
						},
					},
				})
			})

			// System settings — admin role only
			r.With(authmw.RequireRole("admin")).Get("/admin/settings", settingsHandler.List)
			r.With(authmw.RequireRole("admin"), stepUpMFA).Patch("/admin/settings", settingsHandler.Update)

			// OIDC Clients — admin role only
			r.With(authmw.RequireRole("admin")).Get("/admin/oidc/clients", oidcHandler.ListClients)
			r.With(authmw.RequireRole("admin"), stepUpMFA).Post("/admin/oidc/clients", oidcHandler.CreateClient)
			r.With(authmw.RequireRole("admin"), stepUpMFA).Put("/admin/oidc/clients/{id}", oidcHandler.UpdateClient)
			r.With(authmw.RequireRole("admin"), stepUpMFA).Delete("/admin/oidc/clients/{id}", oidcHandler.DeleteClient)
			r.With(authmw.RequireRole("admin"), stepUpMFA).Post("/admin/oidc/clients/{id}/rotate", oidcHandler.RotateClientSecret)

			// Audit log
			r.With(authmw.RequirePermission("audit", "read")).Get("/admin/audit", auditHandler.List)
			r.With(authmw.RequirePermission("audit", "read")).Get("/admin/audit/export", auditHandler.Export)
			r.With(authmw.RequirePermission("audit", "read")).Get("/admin/audit/trends", auditHandler.Trends)
			r.With(authmw.RequirePermission("audit", "read")).Get("/admin/security-dashboard", auditHandler.SecurityDashboard)

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
		return fmt.Errorf("server terminated unexpectedly")
	}
	return nil
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
	DatabaseMaxConns         int
	DatabaseMinConns         int
	DatabaseConnTimeout      time.Duration
	RedisAddr                string
	RedisPassword            string
	RedisPoolSize            int
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
	DPoPExpectedOrigin       string
	WebAuthnRPID             string
	WebAuthnRPDisplayName    string
	GeoIPDBPath              string
	OIDCIssuerURL            string
}

func loadConfig() (config, error) {
	tlsEnabled := getEnv("TLS_ENABLED", "false") == "true"
	cookiesSecure := getEnv("COOKIES_SECURE", "false") == "true"
	registrationEnabled := getEnv("REGISTRATION_ENABLED", "false") == "true"
	mfaEnabled, err := boolEnv("MFA_ENABLED", false)
	if err != nil {
		return config{}, err
	}
	origins, err := parseCORSOrigins(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"))
	if err != nil {
		return config{}, err
	}

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

	dbMaxConns, err := intEnv("DATABASE_MAX_CONNS", 20)
	if err != nil {
		return config{}, err
	}
	dbMinConns, err := intEnv("DATABASE_MIN_CONNS", 2)
	if err != nil {
		return config{}, err
	}
	dbConnTimeoutStr := getEnv("DATABASE_CONN_TIMEOUT", "5s")
	dbConnTimeout, err := time.ParseDuration(dbConnTimeoutStr)
	if err != nil {
		return config{}, fmt.Errorf("invalid DATABASE_CONN_TIMEOUT %q: %w", dbConnTimeoutStr, err)
	}
	redisPoolSize, err := intEnv("REDIS_POOL_SIZE", 10)
	if err != nil {
		return config{}, err
	}

	return config{
		ServerAddr:               getEnv("SERVER_ADDR", ":8080"),
		DatabaseURL:              getEnv("DATABASE_URL", "postgres://zerotrust:zerotrust_secret@localhost:5432/zerotrust_db?sslmode=disable"),
		DatabaseMaxConns:         dbMaxConns,
		DatabaseMinConns:         dbMinConns,
		DatabaseConnTimeout:      dbConnTimeout,
		RedisAddr:                getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:            getEnv("REDIS_PASSWORD", "zerotrust_secret"),
		RedisPoolSize:            redisPoolSize,
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
		DPoPExpectedOrigin:       getEnv("DPOP_EXPECTED_ORIGIN", ""),
		WebAuthnRPID:             getEnv("WEBAUTHN_RP_ID", "localhost"),
		WebAuthnRPDisplayName:    getEnv("WEBAUTHN_RP_DISPLAY_NAME", "ZeroTrust"),
		GeoIPDBPath:              getEnv("GEOIP_DB_PATH", "./GeoLite2-City.mmdb"),
		OIDCIssuerURL:            getEnv("OIDC_ISSUER_URL", "http://localhost:8080"),
	}, nil
}

// parseCORSOrigins splits and trims the configured origins, rejecting the "*"
// wildcard. The server sends credentialed CORS responses (AllowCredentials),
// and a wildcard origin with credentials is both invalid per the Fetch spec and
// a security risk, so it is refused at startup rather than silently mishandled.
// (ISSUE_LIST #39)
func parseCORSOrigins(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		o := strings.TrimSpace(p)
		if o == "" {
			continue
		}
		if o == "*" {
			return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS may not be \"*\" because credentials are enabled; list explicit origins")
		}
		origins = append(origins, o)
	}
	if len(origins) == 0 {
		return nil, fmt.Errorf("CORS_ALLOWED_ORIGINS must contain at least one origin")
	}
	return origins, nil
}

// meResponse is the JSON shape returned by GET /api/v1/me.
type meResponse struct {
	UserID      string   `json:"user_id"`
	Email       string   `json:"email"`
	FirstName   string   `json:"first_name"`
	LastName    string   `json:"last_name"`
	HasAvatar   bool     `json:"has_avatar"`
	Locale      string   `json:"locale"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// buildMeResponse assembles the /me payload, normalizing nil slices to empty
// arrays so the JSON encoder emits [] rather than null. (#37)
func buildMeResponse(profile *user.User, perms []string) meResponse {
	roles := profile.Roles
	if roles == nil {
		roles = []string{}
	}
	if perms == nil {
		perms = []string{}
	}
	return meResponse{
		UserID:      profile.ID,
		Email:       profile.Email,
		FirstName:   profile.FirstName,
		LastName:    profile.LastName,
		HasAvatar:   profile.HasAvatar,
		Locale:      profile.Locale,
		Roles:       roles,
		Permissions: perms,
		CreatedAt:   profile.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   profile.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
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

func intEnv(key string, fallback int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid int env %s=%q: %w", key, v, err)
	}
	return i, nil
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

type serviceAccountLookup interface {
	FindByClientID(ctx context.Context, clientID string) (*serviceaccount.ServiceAccount, error)
	CheckSecret(hash, secret string) bool
}

// saStoreAdapter bridges service-account lookups to auth.ServiceAccountStore.
type saStoreAdapter struct{ svc serviceAccountLookup }

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

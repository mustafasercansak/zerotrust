package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/session"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
	"github.com/zerotrust/backend/pkg/middleware"
)

func TestAuthenticate(t *testing.T) {
	dbURL := testdb.URL(t)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test db unreachable: %v", err)
	}
	if err := database.RunMigrations(dbURL, "../../migrations"); err != nil {
		pool.Close()
		t.Fatalf("migrations failed: %v", err)
	}
	defer pool.Close()

	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	sessionRepo := session.NewRepository(pool, nil)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	authSvc := auth.NewService(userSvc, sessionRepo, nil, rdb, ks, nil, nil)

	handler := middleware.Authenticate(ks, authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := middleware.ClaimsFrom(r.Context())
		if claims != nil {
			w.Write([]byte(claims.UserID))
		}
	}))

	// Missing token
	req1 := httptest.NewRequest("GET", "/", nil)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing token")
	}

	// Invalid token
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("Authorization", "Bearer invalid.token.here")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token")
	}

	// Create real user and token
	pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")
	u, err := userSvc.Register(ctx, "auth_mid@example.com", "passW0rd123!", "en")
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}
	tokenPair, _ := auth.GenerateTokenPair(ks, u.ID, u.Email, "en", u.Roles, []string{}, time.Hour)

	req3 := httptest.NewRequest("GET", "/", nil)
	req3.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("expected 200 for valid token, got %d", rr3.Code)
	}

	sessionRepo.Create(ctx, u.ID, "session1", "127.0.0.1", "test", nil, time.Now().Add(time.Hour))

	req4 := httptest.NewRequest("GET", "/", nil)
	req4.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
	rr4 := httptest.NewRecorder()
	handler.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Errorf("expected 200 for valid session, got %d", rr4.Code)
	}

	if err := authSvc.Logout(ctx, "", tokenPair.AccessToken); err != nil {
		t.Fatalf("logout failed: %v", err)
	}
	rr5 := httptest.NewRecorder()
	handler.ServeHTTP(rr5, req4)
	if rr5.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for revoked session")
	}
}

type mockSettings struct {
	ipAllowlist      string
	countryAllowlist string
	deviceTrust      string
}

func (m *mockSettings) GetInt(_ context.Context, _ string, defaultVal int) int {
	return defaultVal
}
func (m *mockSettings) GetString(_ context.Context, key string, defaultVal string) string {
	if key == "ip_allowlist" {
		return m.ipAllowlist
	}
	if key == "country_allowlist" {
		return m.countryAllowlist
	}
	return defaultVal
}
func (m *mockSettings) GetBool(_ context.Context, key string, defaultVal bool) bool {
	if key == "device_trust_enabled" {
		return m.deviceTrust == "true"
	}
	return defaultVal
}

func TestCAE(t *testing.T) {
	dbURL := testdb.URL(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	defer pool.Close()

	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	sessionRepo := session.NewRepository(pool, nil)
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)

	mockSetts := &mockSettings{}
	authSvc := auth.NewService(userSvc, sessionRepo, nil, rdb, ks, nil, mockSetts)

	handler := middleware.Authenticate(ks, authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Register user
	pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")
	u, err := userSvc.Register(ctx, "cae_test@example.com", "passW0rd123!", "en")
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}
	tokenPair, _ := auth.GenerateTokenPair(ks, u.ID, u.Email, "en", u.Roles, []string{}, time.Hour)

	// 1. Valid user works
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1"
	req.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// 2. User deactivated in DB immediately rejects request
	_, err = pool.Exec(ctx, "UPDATE users SET is_active = false WHERE id = $1", u.ID)
	if err != nil {
		t.Fatalf("failed to deactivate user: %v", err)
	}

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for inactive user, got %d", rr2.Code)
	}
	if !strings.Contains(rr2.Body.String(), "user_inactive") {
		t.Errorf("expected user_inactive error message, got %s", rr2.Body.String())
	}

	// Reactivate user for next checks
	pool.Exec(ctx, "UPDATE users SET is_active = true WHERE id = $1", u.ID)

	// 3. Changed user roles immediately rejects request
	err = userSvc.SetRoles(ctx, u.ID, []string{"admin"})
	if err != nil {
		t.Fatalf("failed to update user roles: %v", err)
	}

	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req)
	if rr3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for changed roles, got %d", rr3.Code)
	}

	// Restore roles
	userSvc.SetRoles(ctx, u.ID, []string{})

	// 4. IP Allowlist change immediately blocks requests
	mockSetts.ipAllowlist = "10.0.0.0/8" // Allow only private range
	rr4 := httptest.NewRecorder()
	handler.ServeHTTP(rr4, req) // coming from 127.0.0.1 (not in 10.0.0.0/8)
	if rr4.Code != http.StatusForbidden {
		t.Errorf("expected 403 for blocked IP allowlist, got %d", rr4.Code)
	}
	if !strings.Contains(rr4.Body.String(), "ip_not_allowed") {
		t.Errorf("expected ip_not_allowed error message, got %s", rr4.Body.String())
	}

	mockSetts.ipAllowlist = "" // reset
}

// TestAuthenticateRejectsOIDCAccessToken is the regression test for the
// audience-confusion finding (#81): an OIDC access token issued to an external
// client (signed by the same KeyStore, carrying Roles, but no first-party
// token_use marker) must be rejected with 401 on internal /api/v1 routes such
// as /me and /webauthn/register/begin, while first-party session and service
// tokens keep working.
func TestAuthenticateRejectsOIDCAccessToken(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	// No user/service stores needed: the token must be rejected before any
	// entity lookup, and first-party tokens skip the lookup when stores are nil.
	authSvc := auth.NewService(nil, nil, nil, rdb, ks, nil, nil)

	handler := middleware.Authenticate(ks, authSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Shape matches what oidc.Service.ExchangeCode mints for external clients.
	oidcClaims := &auth.Claims{
		UserID:  "victim-user",
		Email:   "victim@example.com",
		Roles:   []string{"admin"}, // pre-fix OIDC tokens even embedded roles
		SubType: auth.SubTypeUser,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://issuer.example.com",
			Subject:   "victim-user",
			Audience:  jwt.ClaimStrings{"external-client"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        "oidc-jti-1",
		},
	}
	oidcTok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, oidcClaims)
	oidcTok.Header["kid"] = ks.PrimaryKID()
	oidcTokenStr, err := oidcTok.SignedString(ks.PrimaryKey())
	if err != nil {
		t.Fatalf("sign oidc token: %v", err)
	}

	for _, path := range []string{"/api/v1/me", "/api/v1/webauthn/register/begin"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+oidcTokenStr)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401 for OIDC client access token, got %d", path, rr.Code)
		}
	}

	// First-party session token still passes.
	pair, err := auth.GenerateTokenPair(ks, "victim-user", "victim@example.com", "en", []string{"admin"}, nil, time.Hour)
	if err != nil {
		t.Fatalf("generate session token: %v", err)
	}
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for first-party session token, got %d", rr.Code)
	}

	// First-party service token still passes.
	svcTok, err := auth.GenerateServiceToken(ks, "ci-client", "ci-bot", []string{"users:read"}, time.Hour, "")
	if err != nil {
		t.Fatalf("generate service token: %v", err)
	}
	req2 := httptest.NewRequest("GET", "/api/v1/me", nil)
	req2.Header.Set("Authorization", "Bearer "+svcTok.AccessToken)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200 for first-party service token, got %d", rr2.Code)
	}
}

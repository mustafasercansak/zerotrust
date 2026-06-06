package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
)

func setChiParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestHandler_Discovery(t *testing.T) {
	h := &Handler{issuer: "https://auth.example.com"}
	req, _ := http.NewRequest("GET", "/.well-known/openid-configuration", nil)
	rr := httptest.NewRecorder()

	h.Discovery(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rr.Code)
	}

	var config map[string]any
	json.NewDecoder(rr.Body).Decode(&config)

	if config["issuer"] != "https://auth.example.com" {
		t.Errorf("issuer = %v, want https://auth.example.com", config["issuer"])
	}
	if config["token_endpoint"] != "https://auth.example.com/oauth2/token" {
		t.Errorf("token_endpoint = %v", config["token_endpoint"])
	}

	methods, ok := config["code_challenge_methods_supported"].([]any)
	if !ok || len(methods) != 1 || methods[0] != "S256" {
		t.Errorf("code_challenge_methods_supported = %v, want [S256]", config["code_challenge_methods_supported"])
	}
}

func TestHandler_GetPublicClient_Integration(t *testing.T) {
	dbURL := testdb.URL(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("test db unreachable: %v", err)
	}
	if err := database.RunMigrations(dbURL, "../../migrations"); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}
	pool.Exec(ctx, "TRUNCATE TABLE oauth2_clients CASCADE")

	_, err = pool.Exec(ctx, `
		INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
		VALUES ($1, $2, $3, $4, $5)
	`, "pub-client", "$2a$12$dummyhashvalue000000000000000000000000000000000000000", "Public App", []string{"http://localhost/cb"}, []string{"openid", "email"})
	if err != nil {
		t.Fatalf("insert client: %v", err)
	}

	clientRepo := NewClientRepository(pool)
	h := &Handler{clientRepo: clientRepo}

	// known client returns name + allowed_scopes
	req, _ := http.NewRequestWithContext(ctx, "GET", "/oauth2/clients/pub-client", nil)
	req = setChiParam(req, "client_id", "pub-client")
	rr := httptest.NewRecorder()
	h.GetPublicClient(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["name"] != "Public App" {
		t.Errorf("name = %v, want Public App", resp["name"])
	}
	scopes, _ := resp["allowed_scopes"].([]any)
	if len(scopes) != 2 {
		t.Errorf("allowed_scopes len = %d, want 2", len(scopes))
	}

	// unknown client returns 404
	req2, _ := http.NewRequestWithContext(ctx, "GET", "/oauth2/clients/no-such-client", nil)
	req2 = setChiParam(req2, "client_id", "no-such-client")
	rr2 := httptest.NewRecorder()
	h.GetPublicClient(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown client, got %d", rr2.Code)
	}
}

func TestHandler_OAuthFlow_Integration(t *testing.T) {
	dbURL := testdb.URL(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("test db unreachable: %v", err)
	}

	if err := database.RunMigrations(dbURL, "../../migrations"); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	// Clean tables
	pool.Exec(ctx, "TRUNCATE TABLE oauth2_clients CASCADE")
	pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")

	// Insert test client
	// Secret is "client-secret-123"
	clientSecretHash := "$2a$12$Xi8n6yn8qXuyeVdGn6wfU.Vw2BVtMd/78bk/76tzxeqDvxpJaJEY6"
	_, err = pool.Exec(ctx, `
		INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
		VALUES ($1, $2, $3, $4, $5)
	`, "test-client-id", clientSecretHash, "Test Client", []string{"http://localhost/callback"}, []string{"openid", "profile"})
	if err != nil {
		t.Fatalf("insert client failed: %v", err)
	}

	// Insert test user
	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	u, err := userSvc.Register(ctx, "test-user@example.com", "Password123!", "en")
	if err != nil {
		t.Fatalf("register user failed: %v", err)
	}

	// Miniredis for session store
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, err := auth.LoadOrGenerateKeyStore("", "")
	if err != nil {
		t.Fatalf("keystore load: %v", err)
	}

	clientRepo := NewClientRepository(pool)
	codeStore := NewAuthCodeStore(rdb)
	oidcSvc := NewService(clientRepo, codeStore, userSvc, ks, "http://localhost")
	h := NewHandler(oidcSvc, clientRepo, userSvc, nil, ks, "http://localhost", "http://localhost")

	// Test Authorize handler (redirect to consent because logged in)
	// We mimic a valid token cookie
	tokenStr, _ := auth.GenerateTokenPair(ks, u.ID, u.Email, u.Locale, nil, nil, time.Hour)
	cookie := &http.Cookie{
		Name:  "access_token",
		Value: tokenStr.AccessToken,
	}

	req, _ := http.NewRequest("GET", "/oauth2/authorize?client_id=test-client-id&redirect_uri=http://localhost/callback&response_type=code&scope=openid+profile&state=abc", nil)
	req.AddCookie(cookie)
	// Mock a dummy authSvc to bypass IsRevoked check
	dummyAuthSvc := auth.NewService(userSvc, &mockSessionRepo{}, &testServiceAccountStore{}, rdb, ks, nil, nil)
	h.authSvc = dummyAuthSvc

	rr := httptest.NewRecorder()
	h.Authorize(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("Expected 302 redirect, got %d", rr.Code)
	}

	redirectLocation := rr.Header().Get("Location")
	if !strings.Contains(redirectLocation, "/oauth2/consent") {
		t.Fatalf("Expected redirect to consent, got %q", redirectLocation)
	}

	// Test Consent approval POST
	consentReqBody, _ := json.Marshal(ConsentRequest{
		ClientID:     "test-client-id",
		RedirectURI:  "http://localhost/callback",
		Scopes:       []string{"openid", "profile"},
		State:        "abc",
		Approved:     true,
	})
	consentReq, _ := http.NewRequest("POST", "/oauth2/consent", bytes.NewBuffer(consentReqBody))
	consentReq.AddCookie(cookie)
	consentRR := httptest.NewRecorder()

	h.Consent(consentRR, consentReq)
	if consentRR.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d. Body: %s", consentRR.Code, consentRR.Body.String())
	}

	var consentResp map[string]string
	json.NewDecoder(consentRR.Body).Decode(&consentResp)
	redirectURL := consentResp["redirect_url"]
	if !strings.Contains(redirectURL, "code=") {
		t.Fatalf("Expected redirect URL to contain code, got %s", redirectURL)
	}

	// Extract authorization code from redirect URL
	parsedURL, _ := url.Parse(redirectURL)
	code := parsedURL.Query().Get("code")

	// Test Token exchange POST /oauth2/token
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", "test-client-id")
	form.Set("client_secret", "demo-secret") // matches seeded bcrypt hash
	form.Set("redirect_uri", "http://localhost/callback")

	tokenReq, _ := http.NewRequest("POST", "/oauth2/token", strings.NewReader(form.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRR := httptest.NewRecorder()

	h.Token(tokenRR, tokenReq)
	if tokenRR.Code != http.StatusOK {
		t.Fatalf("Expected 200 on token swap, got %d. Body: %s", tokenRR.Code, tokenRR.Body.String())
	}

	var tokenResp TokenResponse
	json.NewDecoder(tokenRR.Body).Decode(&tokenResp)
	if tokenResp.AccessToken == "" || tokenResp.IDToken == "" {
		t.Fatalf("Expected access and ID tokens, got: %+v", tokenResp)
	}

	// Test UserInfo GET /oauth2/userinfo
	userinfoReq, _ := http.NewRequest("GET", "/oauth2/userinfo", nil)
	userinfoReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userinfoRR := httptest.NewRecorder()

	h.UserInfo(userinfoRR, userinfoReq)
	if userinfoRR.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", userinfoRR.Code)
	}

	var userinfo map[string]any
	json.NewDecoder(userinfoRR.Body).Decode(&userinfo)
	if userinfo["email"] != "test-user@example.com" {
		t.Errorf("UserInfo email = %v, want test-user@example.com", userinfo["email"])
	}
}

// Helpers/Fakes
type mockSessionRepo struct{}
func (m *mockSessionRepo) Create(ctx context.Context, userID, tokenHash, ip, userAgent string, deviceInfo map[string]string, expiresAt time.Time) error {
	return nil
}
func (m *mockSessionRepo) RevokeForDevice(ctx context.Context, userID, ip, userAgent string, deviceInfo map[string]string) error {
	return nil
}
func (m *mockSessionRepo) RotateSession(ctx context.Context, oldHash string, generate func(userID string, lastActiveAt, currentExpiresAt time.Time) (newHash, ip, ua string, deviceInfo map[string]string, expiresAt time.Time, err error)) error {
	return nil
}
func (m *mockSessionRepo) Revoke(ctx context.Context, hash string) error {
	return nil
}
func (m *mockSessionRepo) EvictExcessSessions(ctx context.Context, userID string, keep int) error {
	return nil
}
func (m *mockSessionRepo) CheckReuse(ctx context.Context, hash string) (string, error) {
	return "", nil
}
func (m *mockSessionRepo) RevokeAllForUser(ctx context.Context, userID string) error {
	return nil
}

type testServiceAccountStore struct{}
func (m *testServiceAccountStore) FindByClientID(ctx context.Context, clientID string) (*auth.ServiceAccountRecord, error) {
	return nil, nil
}
func (m *testServiceAccountStore) CheckSecret(hash, secret string) bool {
	return false
}

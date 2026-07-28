package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"sync"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/audit"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
	authmw "github.com/zerotrust/backend/pkg/middleware"
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
	if config["revocation_endpoint"] != "https://auth.example.com/oauth2/revoke" {
		t.Errorf("revocation_endpoint = %v", config["revocation_endpoint"])
	}
	if config["introspection_endpoint"] != "https://auth.example.com/oauth2/introspect" {
		t.Errorf("introspection_endpoint = %v", config["introspection_endpoint"])
	}
	methods, ok := config["code_challenge_methods_supported"].([]any)
	if !ok || len(methods) != 1 || methods[0] != "S256" {
		t.Errorf("code_challenge_methods_supported = %v, want [S256]", config["code_challenge_methods_supported"])
	}
	grantTypes, ok := config["grant_types_supported"].([]any)
	if !ok {
		t.Errorf("grant_types_supported missing or wrong type: %v", config["grant_types_supported"])
	} else {
		gtMap := make(map[string]bool)
		for _, g := range grantTypes {
			if s, ok := g.(string); ok {
				gtMap[s] = true
			}
		}
		if !gtMap["authorization_code"] || !gtMap["refresh_token"] {
			t.Errorf("grant_types_supported = %v, want [authorization_code refresh_token]", grantTypes)
		}
	}

	scopes, ok := config["scopes_supported"].([]any)
	if !ok {
		t.Errorf("scopes_supported missing or wrong type: %v", config["scopes_supported"])
	} else {
		scopeMap := make(map[string]bool)
		for _, s := range scopes {
			if str, ok := s.(string); ok {
				scopeMap[str] = true
			}
		}
		for _, want := range []string{"openid", "profile", "email", "offline_access"} {
			if !scopeMap[want] {
				t.Errorf("scopes_supported missing %q, got %v", want, scopes)
			}
		}
	}

	prompts, ok := config["prompt_values_supported"].([]any)
	if !ok {
		t.Errorf("prompt_values_supported missing or wrong type: %v", config["prompt_values_supported"])
	} else {
		promptMap := make(map[string]bool)
		for _, p := range prompts {
			if str, ok := p.(string); ok {
				promptMap[str] = true
			}
		}
		for _, want := range []string{"none", "login", "consent"} {
			if !promptMap[want] {
				t.Errorf("prompt_values_supported missing %q", want)
			}
		}
	}

	if config["request_uri_parameter_supported"] != false {
		t.Errorf("request_uri_parameter_supported = %v, want false", config["request_uri_parameter_supported"])
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
	`, "test-client-id", clientSecretHash, "Test Client", []string{"http://localhost/callback"}, []string{"openid", "profile", "email"})
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

	ks, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	if err != nil {
		t.Fatalf("keystore load: %v", err)
	}

	clientRepo := NewClientRepository(pool)
	codeStore := NewAuthCodeStore(rdb)
	oidcSvc := NewService(clientRepo, codeStore, userSvc, ks, "http://localhost", nil)
	h := NewHandler(oidcSvc, clientRepo, userSvc, nil, ks, "http://localhost", "http://localhost", nil, nil, nil)

	// Test Authorize handler (redirect to consent because logged in)
	// We mimic a valid token cookie
	tokenStr, _ := auth.GenerateTokenPair(ks, u.ID, u.Email, u.Locale, nil, nil, time.Hour)
	cookie := &http.Cookie{
		Name:  "access_token",
		Value: tokenStr.AccessToken,
	}

	req, _ := http.NewRequest("GET", "/oauth2/authorize?client_id=test-client-id&redirect_uri=http://localhost/callback&response_type=code&scope=openid+profile+email&state=abc", nil)
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
		Scopes:       []string{"openid", "profile", "email"},
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

func TestHandler_Revoke_Integration(t *testing.T) {
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
	pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")

	clientSecretHash := "$2a$12$Xi8n6yn8qXuyeVdGn6wfU.Vw2BVtMd/78bk/76tzxeqDvxpJaJEY6"
	_, err = pool.Exec(ctx, `
		INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
		VALUES ($1, $2, $3, $4, $5)
	`, "revoke-client", clientSecretHash, "Revoke Test", []string{"http://localhost/cb"}, []string{"openid"})
	if err != nil {
		t.Fatalf("insert client: %v", err)
	}

	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	u, err := userSvc.Register(ctx, "revoke-user@example.com", "Password123!", "en")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}

	clientRepo := NewClientRepository(pool)
	codeStore := NewAuthCodeStore(rdb)
	oidcSvc := NewService(clientRepo, codeStore, userSvc, ks, "http://localhost", nil)
	authSvc := auth.NewService(userSvc, &mockSessionRepo{}, &testServiceAccountStore{}, rdb, ks, nil, nil)
	h := NewHandler(oidcSvc, clientRepo, userSvc, authSvc, ks, "http://localhost", "http://localhost", nil, nil, nil)

	// Issue a token via the full flow
	tokenPair, _ := auth.GenerateTokenPair(ks, u.ID, u.Email, u.Locale, nil, nil, time.Hour)
	cookie := &http.Cookie{Name: "access_token", Value: tokenPair.AccessToken}

	// Consent → get auth code
	consentBody, _ := json.Marshal(ConsentRequest{
		ClientID: "revoke-client", RedirectURI: "http://localhost/cb",
		Scopes: []string{"openid"}, State: "s1", Approved: true,
	})
	consentReq, _ := http.NewRequest("POST", "/oauth2/consent", bytes.NewBuffer(consentBody))
	consentReq.AddCookie(cookie)
	consentRR := httptest.NewRecorder()
	h.Consent(consentRR, consentReq)
	if consentRR.Code != http.StatusOK {
		t.Fatalf("consent: got %d: %s", consentRR.Code, consentRR.Body.String())
	}
	var consentResp map[string]string
	json.NewDecoder(consentRR.Body).Decode(&consentResp)
	parsed, _ := url.Parse(consentResp["redirect_url"])
	code := parsed.Query().Get("code")

	// Token exchange → access token
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", "revoke-client")
	form.Set("client_secret", "demo-secret")
	form.Set("redirect_uri", "http://localhost/cb")
	tokenReq, _ := http.NewRequest("POST", "/oauth2/token", strings.NewReader(form.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRR := httptest.NewRecorder()
	h.Token(tokenRR, tokenReq)
	if tokenRR.Code != http.StatusOK {
		t.Fatalf("token exchange: got %d: %s", tokenRR.Code, tokenRR.Body.String())
	}
	var tokenResp TokenResponse
	json.NewDecoder(tokenRR.Body).Decode(&tokenResp)
	accessToken := tokenResp.AccessToken

	// Confirm UserInfo works before revocation
	uiReq, _ := http.NewRequest("GET", "/oauth2/userinfo", nil)
	uiReq.Header.Set("Authorization", "Bearer "+accessToken)
	uiRR := httptest.NewRecorder()
	h.UserInfo(uiRR, uiReq)
	if uiRR.Code != http.StatusOK {
		t.Fatalf("userinfo before revoke: got %d", uiRR.Code)
	}

	// Revoke the token
	revokeForm := url.Values{}
	revokeForm.Set("token", accessToken)
	revokeForm.Set("client_id", "revoke-client")
	revokeForm.Set("client_secret", "demo-secret")
	revokeReq, _ := http.NewRequest("POST", "/oauth2/revoke", strings.NewReader(revokeForm.Encode()))
	revokeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	revokeRR := httptest.NewRecorder()
	h.Revoke(revokeRR, revokeReq)
	if revokeRR.Code != http.StatusOK {
		t.Fatalf("revoke: got %d", revokeRR.Code)
	}

	// UserInfo must now return 401
	uiReq2, _ := http.NewRequest("GET", "/oauth2/userinfo", nil)
	uiReq2.Header.Set("Authorization", "Bearer "+accessToken)
	uiRR2 := httptest.NewRecorder()
	h.UserInfo(uiRR2, uiReq2)
	if uiRR2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after revocation, got %d", uiRR2.Code)
	}

	// Revoke with wrong client secret must return 401
	badForm := url.Values{}
	badForm.Set("token", accessToken)
	badForm.Set("client_id", "revoke-client")
	badForm.Set("client_secret", "wrong-secret")
	badReq, _ := http.NewRequest("POST", "/oauth2/revoke", strings.NewReader(badForm.Encode()))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badRR := httptest.NewRecorder()
	h.Revoke(badRR, badReq)
	if badRR.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for bad client secret, got %d", badRR.Code)
	}
}

func TestHandler_AuditLogging_Consent(t *testing.T) {
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
	pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")

	clientSecretHash := "$2a$12$Xi8n6yn8qXuyeVdGn6wfU.Vw2BVtMd/78bk/76tzxeqDvxpJaJEY6"
	pool.Exec(ctx, `INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
		VALUES ($1, $2, $3, $4, $5)`,
		"audit-client", clientSecretHash, "Audit Client", []string{"http://localhost/cb"}, []string{"openid"})

	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	u, err := userSvc.Register(ctx, "audit-user@example.com", "Password123!", "en")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	clientRepo := NewClientRepository(pool)
	codeStore := NewAuthCodeStore(rdb)
	oidcSvc := NewService(clientRepo, codeStore, userSvc, ks, "http://localhost", nil)

	al := &mockAuditLogger{}
	h := NewHandler(oidcSvc, clientRepo, userSvc, nil, ks, "http://localhost", "http://localhost", al, nil, nil)

	tokenPair, _ := auth.GenerateTokenPair(ks, u.ID, u.Email, u.Locale, nil, nil, time.Hour)
	cookie := &http.Cookie{Name: "access_token", Value: tokenPair.AccessToken}

	// Denied consent must log oidc.consent_denied
	denyBody, _ := json.Marshal(ConsentRequest{
		ClientID: "audit-client", RedirectURI: "http://localhost/cb",
		Scopes: []string{"openid"}, Approved: false,
	})
	denyReq, _ := http.NewRequest("POST", "/oauth2/consent", bytes.NewBuffer(denyBody))
	denyReq.AddCookie(cookie)
	h.Consent(httptest.NewRecorder(), denyReq)

	// Give the goroutine a moment to fire
	time.Sleep(50 * time.Millisecond)
	if !al.hasAction("oidc.consent_denied") {
		t.Errorf("expected oidc.consent_denied audit entry, got: %v", al.actions())
	}

	// Approved consent must log oidc.consent_approved
	approveBody, _ := json.Marshal(ConsentRequest{
		ClientID: "audit-client", RedirectURI: "http://localhost/cb",
		Scopes: []string{"openid"}, Approved: true,
	})
	approveReq, _ := http.NewRequest("POST", "/oauth2/consent", bytes.NewBuffer(approveBody))
	approveReq.AddCookie(cookie)
	h.Consent(httptest.NewRecorder(), approveReq)

	time.Sleep(50 * time.Millisecond)
	if !al.hasAction("oidc.consent_approved") {
		t.Errorf("expected oidc.consent_approved audit entry, got: %v", al.actions())
	}
}

func TestHandler_AuditLogging_RotateSecret(t *testing.T) {
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

	var clientUUID string
	err = pool.QueryRow(ctx, `
		INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, "audit-rotate-client", "$2a$12$Xi8n6yn8qXuyeVdGn6wfU.Vw2BVtMd/78bk/76tzxeqDvxpJaJEY6", "Audit Rotate", []string{}, []string{"openid"}).Scan(&clientUUID)
	if err != nil {
		t.Fatalf("insert client: %v", err)
	}

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	adminID := "admin-user-uuid"
	adminToken, _ := auth.GenerateTokenPair(ks, adminID, "admin@example.com", "en", []string{"admin"}, nil, time.Hour)

	clientRepo := NewClientRepository(pool)
	al := &mockAuditLogger{}
	h := &Handler{clientRepo: clientRepo, auditRepo: al, ks: ks}

	req := setChiParam(httptest.NewRequest("POST", "/admin/oidc/clients/"+clientUUID+"/rotate", nil), "id", clientUUID)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken.AccessToken})
	ctx2 := context.WithValue(req.Context(), authmw.ClaimsKey, &auth.Claims{UserID: adminID})
	req = req.WithContext(ctx2)

	h.RotateClientSecret(httptest.NewRecorder(), req)

	time.Sleep(50 * time.Millisecond)
	if !al.hasAction("oidc.client_secret_rotated") {
		t.Errorf("expected oidc.client_secret_rotated audit entry, got: %v", al.actions())
	}
}

func TestHandler_AuditLogging_Introspect(t *testing.T) {
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
	pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")

	clientSecretHash := "$2a$12$Xi8n6yn8qXuyeVdGn6wfU.Vw2BVtMd/78bk/76tzxeqDvxpJaJEY6"
	_, err = pool.Exec(ctx, `
		INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
		VALUES ($1, $2, $3, $4, $5)
	`, "audit-intro-client", clientSecretHash, "Audit Intro", []string{}, []string{"openid"})
	if err != nil {
		t.Fatalf("insert client: %v", err)
	}

	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	u, err := userSvc.Register(ctx, "audit-intro-user@example.com", "Password123!", "en")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	authSvc := auth.NewService(userSvc, &mockSessionRepo{}, &testServiceAccountStore{}, rdb, ks, nil, nil)
	clientRepo := NewClientRepository(pool)
	al := &mockAuditLogger{}
	h := &Handler{ks: ks, authSvc: authSvc, clientRepo: clientRepo, auditRepo: al}

	tokenPair, _ := auth.GenerateTokenPair(ks, u.ID, u.Email, u.Locale, nil, nil, time.Hour)

	// Active token — should log active=true
	form := url.Values{}
	form.Set("client_id", "audit-intro-client")
	form.Set("client_secret", "demo-secret")
	form.Set("token", tokenPair.AccessToken)
	req, _ := http.NewRequest("POST", "/oauth2/introspect", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.Introspect(httptest.NewRecorder(), req)

	time.Sleep(50 * time.Millisecond)
	if !al.hasAction("oidc.token_introspected") {
		t.Errorf("expected oidc.token_introspected audit entry, got: %v", al.actions())
	}

	// Expired/invalid token — should also log active=false
	al.mu.Lock()
	al.entries = nil
	al.mu.Unlock()

	form2 := url.Values{}
	form2.Set("client_id", "audit-intro-client")
	form2.Set("client_secret", "demo-secret")
	form2.Set("token", "not.a.valid.token")
	req2, _ := http.NewRequest("POST", "/oauth2/introspect", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	h.Introspect(httptest.NewRecorder(), req2)

	time.Sleep(50 * time.Millisecond)
	if !al.hasAction("oidc.token_introspected") {
		t.Errorf("expected oidc.token_introspected audit entry for inactive token, got: %v", al.actions())
	}
}

func TestHandler_Revoke_RefreshToken(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	refreshStore := NewRefreshTokenStore(rdb)
	svc := NewService(nil, nil, nil, ks, "http://localhost", refreshStore)

	// Seed a refresh token directly into the store; we bypass client auth by
	// using a stub clientRepo that always authenticates successfully.
	ctx := context.Background()
	sess := &OIDCRefreshSession{UserID: "u1", ClientID: "stub-client", Scopes: []string{"openid", "offline_access"}, AuthTime: time.Now()}
	rt, _ := refreshStore.Save(ctx, sess)

	h := &Handler{
		svc:        svc,
		ks:         ks,
		clientRepo: &stubClientRepo{},
	}

	// Revoke via token_type_hint=refresh_token
	form := url.Values{}
	form.Set("client_id", "stub-client")
	form.Set("client_secret", "any")
	form.Set("token", rt)
	form.Set("token_type_hint", "refresh_token")
	req, _ := http.NewRequest("POST", "/oauth2/revoke", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Revoke(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// The token must now be gone from the store
	_, err = refreshStore.GetAndConsume(ctx, rt)
	if err != ErrRefreshTokenNotFound {
		t.Errorf("expected ErrRefreshTokenNotFound after revocation, got %v", err)
	}
}

// stubClientRepo always authenticates any client successfully — used in unit
// tests that don't need real DB client authentication. RedirectURIs is
// optional; set it when the test exercises redirect-URI validation.
type stubClientRepo struct {
	redirectURIs []string
}

func (s *stubClientRepo) FindByClientID(_ context.Context, clientID string) (*Client, error) {
	return &Client{ClientID: clientID, RedirectURIs: s.redirectURIs}, nil
}
func (s *stubClientRepo) AuthenticateClient(_ context.Context, clientID, _ string) (*Client, error) {
	return &Client{ClientID: clientID}, nil
}
func (s *stubClientRepo) List(_ context.Context) ([]*Client, error)  { return nil, nil }
func (s *stubClientRepo) Create(_ context.Context, clientID, _, name string, uris, scopes []string) (*Client, error) {
	return &Client{ClientID: clientID, Name: name, RedirectURIs: uris, AllowedScopes: scopes}, nil
}
func (s *stubClientRepo) Delete(_ context.Context, _ string) error { return nil }
func (s *stubClientRepo) Update(_ context.Context, id, name string, uris, scopes []string) (*Client, error) {
	return &Client{ID: id, Name: name, RedirectURIs: uris, AllowedScopes: scopes}, nil
}
func (s *stubClientRepo) RotateSecret(_ context.Context, _ string) (string, error) {
	return "stub-secret", nil
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

type mockAuditLogger struct {
	mu      sync.Mutex
	entries []string
}

func (m *mockAuditLogger) Log(_ context.Context, e audit.Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, e.Action)
	return nil
}

func (m *mockAuditLogger) hasAction(action string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Contains(m.entries, action)
}

func (m *mockAuditLogger) actions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.entries...)
}

// mockMFAChecker satisfies consentMFAGuard for unit tests.
type mockMFAChecker struct{ enabled bool }

func (m *mockMFAChecker) IsEnabled(_ context.Context, _ string) bool { return m.enabled }

func TestHandler_Consent_RequiresStepUpWhenMFAEnabled(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	h := &Handler{
		ks:     ks,
		mfaSvc: &mockMFAChecker{enabled: true},
		rdb:    rdb,
	}

	tokenPair, _ := auth.GenerateTokenPair(ks, "user-1", "u@example.com", "en", nil, nil, time.Hour)
	cookie := &http.Cookie{Name: "access_token", Value: tokenPair.AccessToken}

	body, _ := json.Marshal(ConsentRequest{
		ClientID:    "some-client",
		RedirectURI: "http://localhost/cb",
		Scopes:      []string{"openid"},
		State:       "s",
		Approved:    true,
	})
	req, _ := http.NewRequest("POST", "/oauth2/consent", bytes.NewBuffer(body))
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()

	h.Consent(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 mfa_required, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandler_RotateClientSecret_Integration(t *testing.T) {
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

	var clientUUID string
	err = pool.QueryRow(ctx, `
		INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, "rotate-client", "$2a$12$Xi8n6yn8qXuyeVdGn6wfU.Vw2BVtMd/78bk/76tzxeqDvxpJaJEY6", "Rotate Test", []string{}, []string{"openid"}).Scan(&clientUUID)
	if err != nil {
		t.Fatalf("insert client: %v", err)
	}

	clientRepo := NewClientRepository(pool)
	h := &Handler{clientRepo: clientRepo}

	req := setChiParam(httptest.NewRequest("POST", "/admin/oidc/clients/"+clientUUID+"/rotate", nil), "id", clientUUID)
	rr := httptest.NewRecorder()
	h.RotateClientSecret(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp["client_secret"]) < 10 {
		t.Fatalf("expected non-empty client_secret, got %q", resp["client_secret"])
	}
}

func TestHandler_Introspect_Integration(t *testing.T) {
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
	pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")

	// Secret is "introspect-secret"
	clientSecretHash := "$2a$12$Xi8n6yn8qXuyeVdGn6wfU.Vw2BVtMd/78bk/76tzxeqDvxpJaJEY6"
	_, err = pool.Exec(ctx, `
		INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
		VALUES ($1, $2, $3, $4, $5)
	`, "intro-client", clientSecretHash, "Introspect Client", []string{}, []string{"openid"})
	if err != nil {
		t.Fatalf("insert client: %v", err)
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	u, err := userSvc.Register(ctx, "intro-user@example.com", "Password123!", "en")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	authSvc := auth.NewService(userSvc, &mockSessionRepo{}, &testServiceAccountStore{}, rdb, ks, nil, nil)
	clientRepo := NewClientRepository(pool)
	h := &Handler{ks: ks, authSvc: authSvc, clientRepo: clientRepo}

	tokenPair, _ := auth.GenerateTokenPair(ks, u.ID, u.Email, u.Locale, nil, nil, time.Hour)

	// Active token via form params
	form := url.Values{}
	form.Set("client_id", "intro-client")
	form.Set("client_secret", "demo-secret")
	form.Set("token", tokenPair.AccessToken)
	req, _ := http.NewRequest("POST", "/oauth2/introspect", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Introspect(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["active"] != true {
		t.Fatalf("expected active=true, got %v", resp["active"])
	}
	if resp["sub"] != u.ID {
		t.Fatalf("expected sub=%s, got %v", u.ID, resp["sub"])
	}

	// Bad client secret → 401
	form2 := url.Values{}
	form2.Set("client_id", "intro-client")
	form2.Set("client_secret", "wrong-secret")
	form2.Set("token", tokenPair.AccessToken)
	req2, _ := http.NewRequest("POST", "/oauth2/introspect", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rr2 := httptest.NewRecorder()
	h.Introspect(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr2.Code)
	}
}

func TestHandler_RefreshToken_Integration(t *testing.T) {
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
	pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")

	// Secret is "client-secret-123"
	clientSecretHash := "$2a$12$Xi8n6yn8qXuyeVdGn6wfU.Vw2BVtMd/78bk/76tzxeqDvxpJaJEY6"
	_, err = pool.Exec(ctx, `
		INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
		VALUES ($1, $2, $3, $4, $5)
	`, "refresh-client", clientSecretHash, "Refresh Client", []string{"http://localhost/cb"}, []string{"openid", "profile", "offline_access"})
	if err != nil {
		t.Fatalf("insert client: %v", err)
	}

	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo)
	u, err := userSvc.Register(ctx, "refresh-user@example.com", "Password123!", "en")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	clientRepo := NewClientRepository(pool)
	codeStore := NewAuthCodeStore(rdb)
	refreshStore := NewRefreshTokenStore(rdb)
	oidcSvc := NewService(clientRepo, codeStore, userSvc, ks, "http://localhost", refreshStore)
	h := NewHandler(oidcSvc, clientRepo, userSvc, nil, ks, "http://localhost", "http://localhost", nil, nil, nil)

	// Seed an auth code session and exchange it to obtain a refresh token
	tokenPair, _ := auth.GenerateTokenPair(ks, u.ID, u.Email, u.Locale, nil, nil, time.Hour)
	cookie := &http.Cookie{Name: "access_token", Value: tokenPair.AccessToken}

	consentBody, _ := json.Marshal(ConsentRequest{
		ClientID:    "refresh-client",
		RedirectURI: "http://localhost/cb",
		Scopes:      []string{"openid", "profile", "offline_access"},
		Approved:    true,
	})
	consentReq, _ := http.NewRequest("POST", "/oauth2/consent", bytes.NewBuffer(consentBody))
	consentReq.AddCookie(cookie)
	consentRR := httptest.NewRecorder()
	h.Consent(consentRR, consentReq)
	if consentRR.Code != http.StatusOK {
		t.Fatalf("consent: expected 200, got %d: %s", consentRR.Code, consentRR.Body.String())
	}

	var consentResp map[string]string
	json.NewDecoder(consentRR.Body).Decode(&consentResp)
	parsedURL, _ := url.Parse(consentResp["redirect_url"])
	code := parsedURL.Query().Get("code")
	if code == "" {
		t.Fatal("no auth code in consent redirect")
	}

	codeForm := url.Values{}
	codeForm.Set("grant_type", "authorization_code")
	codeForm.Set("code", code)
	codeForm.Set("client_id", "refresh-client")
	codeForm.Set("client_secret", "demo-secret")
	codeForm.Set("redirect_uri", "http://localhost/cb")
	codeReq, _ := http.NewRequest("POST", "/oauth2/token", strings.NewReader(codeForm.Encode()))
	codeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	codeRR := httptest.NewRecorder()
	h.Token(codeRR, codeReq)
	if codeRR.Code != http.StatusOK {
		t.Fatalf("code exchange: expected 200, got %d: %s", codeRR.Code, codeRR.Body.String())
	}

	var firstResp TokenResponse
	json.NewDecoder(codeRR.Body).Decode(&firstResp)
	if firstResp.AccessToken == "" {
		t.Fatal("expected access_token from code exchange")
	}
	if firstResp.RefreshToken == "" {
		t.Fatal("expected refresh_token from code exchange")
	}

	// Exchange the refresh token for a new token pair
	rtForm := url.Values{}
	rtForm.Set("grant_type", "refresh_token")
	rtForm.Set("refresh_token", firstResp.RefreshToken)
	rtForm.Set("client_id", "refresh-client")
	rtForm.Set("client_secret", "demo-secret")
	rtReq, _ := http.NewRequest("POST", "/oauth2/token", strings.NewReader(rtForm.Encode()))
	rtReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rtRR := httptest.NewRecorder()
	h.Token(rtRR, rtReq)
	if rtRR.Code != http.StatusOK {
		t.Fatalf("refresh exchange: expected 200, got %d: %s", rtRR.Code, rtRR.Body.String())
	}

	var secondResp TokenResponse
	json.NewDecoder(rtRR.Body).Decode(&secondResp)
	if secondResp.AccessToken == "" {
		t.Fatal("expected access_token from refresh exchange")
	}
	if secondResp.RefreshToken == "" {
		t.Fatal("expected rotated refresh_token from refresh exchange")
	}
	if secondResp.RefreshToken == firstResp.RefreshToken {
		t.Error("rotated refresh_token must differ from original")
	}

	// Original refresh token must be consumed
	rtForm2 := url.Values{}
	rtForm2.Set("grant_type", "refresh_token")
	rtForm2.Set("refresh_token", firstResp.RefreshToken)
	rtForm2.Set("client_id", "refresh-client")
	rtForm2.Set("client_secret", "demo-secret")
	rtReq2, _ := http.NewRequest("POST", "/oauth2/token", strings.NewReader(rtForm2.Encode()))
	rtReq2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rtRR2 := httptest.NewRecorder()
	h.Token(rtRR2, rtReq2)
	if rtRR2.Code != http.StatusBadRequest {
		t.Errorf("replayed refresh token: expected 400, got %d", rtRR2.Code)
	}
}

// TestHandler_Authorize_Prompt covers the prompt=none and prompt=login values
// (OIDC Core §3.1.2.1).
func TestHandler_Authorize_Prompt(t *testing.T) {
	ks, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}
	h := &Handler{
		ks:           ks,
		publicAppURL: "http://localhost:3000",
		clientRepo:   &stubClientRepo{redirectURIs: []string{"http://localhost/cb"}},
	}

	tokenPair, _ := auth.GenerateTokenPair(ks, "user-1", "u@example.com", "en", nil, nil, time.Hour)
	cookie := &http.Cookie{Name: "access_token", Value: tokenPair.AccessToken}

	// prompt=none, not logged in → redirect to redirect_uri with error=login_required
	req, _ := http.NewRequest("GET", "/oauth2/authorize?client_id=stub-client&redirect_uri=http://localhost/cb&response_type=code&prompt=none&state=s1", nil)
	rr := httptest.NewRecorder()
	h.Authorize(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("prompt=none unauthenticated: expected 302, got %d", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "error=login_required") {
		t.Errorf("prompt=none unauthenticated: expected login_required in Location, got %q", loc)
	}
	if !strings.Contains(loc, "state=s1") {
		t.Errorf("prompt=none unauthenticated: expected state=s1 in Location, got %q", loc)
	}

	// prompt=none, logged in → interaction_required (we always need consent)
	req2, _ := http.NewRequest("GET", "/oauth2/authorize?client_id=stub-client&redirect_uri=http://localhost/cb&response_type=code&prompt=none", nil)
	req2.AddCookie(cookie)
	rr2 := httptest.NewRecorder()
	h.Authorize(rr2, req2)
	if rr2.Code != http.StatusFound {
		t.Fatalf("prompt=none authenticated: expected 302, got %d", rr2.Code)
	}
	loc2 := rr2.Header().Get("Location")
	if !strings.Contains(loc2, "error=interaction_required") {
		t.Errorf("prompt=none authenticated: expected interaction_required in Location, got %q", loc2)
	}

	// prompt=login, logged in → force redirect to login (ignore existing session)
	req3, _ := http.NewRequest("GET", "/oauth2/authorize?client_id=stub-client&redirect_uri=http://localhost/cb&response_type=code&prompt=login", nil)
	req3.AddCookie(cookie)
	rr3 := httptest.NewRecorder()
	h.Authorize(rr3, req3)
	if rr3.Code != http.StatusFound {
		t.Fatalf("prompt=login: expected 302, got %d", rr3.Code)
	}
	loc3 := rr3.Header().Get("Location")
	if !strings.Contains(loc3, "/auth/login") {
		t.Errorf("prompt=login: expected login redirect, got %q", loc3)
	}

	// prompt=none + max_age=0 (token is already old enough to fail) → login_required
	req4, _ := http.NewRequest("GET", "/oauth2/authorize?client_id=stub-client&redirect_uri=http://localhost/cb&response_type=code&prompt=none&max_age=0", nil)
	req4.AddCookie(cookie)
	rr4 := httptest.NewRecorder()
	h.Authorize(rr4, req4)
	if rr4.Code != http.StatusFound {
		t.Fatalf("prompt=none+max_age=0: expected 302, got %d", rr4.Code)
	}
	loc4 := rr4.Header().Get("Location")
	if !strings.Contains(loc4, "error=login_required") {
		t.Errorf("prompt=none+max_age=0: expected login_required, got %q", loc4)
	}
}

// TestHandler_Authorize_MaxAge verifies that a valid session is rejected when
// its age exceeds the max_age parameter (OIDC Core §3.1.2.1).
func TestHandler_Authorize_MaxAge(t *testing.T) {
	ks, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}

	h := &Handler{
		ks:           ks,
		publicAppURL: "http://localhost:3000",
		clientRepo:   &stubClientRepo{redirectURIs: []string{"http://localhost/cb"}},
	}

	// Issue a token stamped with iat = now, so it is 0 seconds old.
	// max_age=0 means "must have just authenticated" — any non-zero age fails.
	tokenPair, _ := auth.GenerateTokenPair(ks, "user-1", "u@example.com", "en", nil, nil, time.Hour)
	cookie := &http.Cookie{Name: "access_token", Value: tokenPair.AccessToken}

	// max_age=0: even a freshly-issued token is >0 seconds old by the time we
	// check; use max_age=99999 (≈27h) to confirm the happy path first.
	req, _ := http.NewRequest("GET", "/oauth2/authorize?client_id=stub-client&redirect_uri=http://localhost/cb&response_type=code&max_age=99999", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	h.Authorize(rr, req)
	// Should proceed to consent redirect, not back to login.
	if rr.Code != http.StatusFound {
		t.Fatalf("happy path: expected 302, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "/oauth2/consent") {
		t.Errorf("happy path: expected consent redirect, got %q", loc)
	}

	// max_age=0: force re-auth (token is always at least a few ms old).
	req2, _ := http.NewRequest("GET", "/oauth2/authorize?client_id=stub-client&redirect_uri=http://localhost/cb&response_type=code&max_age=0", nil)
	req2.AddCookie(cookie)
	rr2 := httptest.NewRecorder()
	h.Authorize(rr2, req2)
	if rr2.Code != http.StatusFound {
		t.Fatalf("max_age=0: expected 302, got %d", rr2.Code)
	}
	if loc := rr2.Header().Get("Location"); !strings.Contains(loc, "/auth/login") {
		t.Errorf("max_age=0: expected login redirect, got %q", loc)
	}
}

// TestHandler_EndSession covers RP-Initiated Logout (OpenID Connect Session
// Management §5): cookie clearing, token revocation, redirect target selection,
// and the id_token_hint / post_logout_redirect_uri validation.
func TestHandler_EndSession(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}

	userSvc := user.NewService(&mockUserReader{
		user: &user.User{ID: "u1", Email: "u@example.com", Locale: "en"},
	})
	authSvc := auth.NewService(userSvc, &mockSessionRepo{}, &testServiceAccountStore{}, rdb, ks, nil, nil)

	h := &Handler{
		ks:           ks,
		authSvc:      authSvc,
		publicAppURL: "http://localhost:3000",
		clientRepo:   &stubClientRepo{redirectURIs: []string{"http://localhost/post-logout"}},
	}

	tokenPair, _ := auth.GenerateTokenPair(ks, "u1", "u@example.com", "en", nil, nil, time.Hour)

	// ── Case 1: no cookies, no hints → redirect to login ────────────────────
	req1, _ := http.NewRequest("GET", "/oauth2/end_session", nil)
	rr1 := httptest.NewRecorder()
	h.EndSession(rr1, req1)
	if rr1.Code != http.StatusFound {
		t.Fatalf("case1: expected 302, got %d", rr1.Code)
	}
	if loc := rr1.Header().Get("Location"); !strings.Contains(loc, "/auth/login") {
		t.Errorf("case1: expected login redirect, got %q", loc)
	}

	// ── Case 2: access_token cookie set → cookies cleared ───────────────────
	req2, _ := http.NewRequest("GET", "/oauth2/end_session", nil)
	req2.AddCookie(&http.Cookie{Name: "access_token", Value: tokenPair.AccessToken})
	req2.AddCookie(&http.Cookie{Name: "refresh_token", Value: "some-refresh"})
	rr2 := httptest.NewRecorder()
	h.EndSession(rr2, req2)
	if rr2.Code != http.StatusFound {
		t.Fatalf("case2: expected 302, got %d", rr2.Code)
	}
	// All four cookies must be expired (MaxAge=-1).
	cleared := make(map[string]bool)
	for _, c := range rr2.Result().Cookies() {
		if c.MaxAge == -1 {
			cleared[c.Name] = true
		}
	}
	for _, name := range []string{"access_token", "refresh_token", "csrf_token", "at_exp"} {
		if !cleared[name] {
			t.Errorf("case2: cookie %q was not cleared", name)
		}
	}

	// ── Case 3: valid id_token_hint + matching post_logout_redirect_uri + state
	// Build a real ID token (same format as ExchangeCode produces).
	idToken := buildIDToken(t, ks, "u1", "stub-client")

	req3, _ := http.NewRequest("GET",
		"/oauth2/end_session?id_token_hint="+url.QueryEscape(idToken)+
			"&post_logout_redirect_uri=http://localhost/post-logout&state=xyz",
		nil)
	rr3 := httptest.NewRecorder()
	h.EndSession(rr3, req3)
	if rr3.Code != http.StatusFound {
		t.Fatalf("case3: expected 302, got %d", rr3.Code)
	}
	loc3 := rr3.Header().Get("Location")
	if !strings.Contains(loc3, "http://localhost/post-logout") {
		t.Errorf("case3: expected post-logout redirect, got %q", loc3)
	}
	if !strings.Contains(loc3, "state=xyz") {
		t.Errorf("case3: expected state=xyz in redirect, got %q", loc3)
	}

	// ── Case 4: valid hint but redirect URI not in client's allowlist ────────
	req4, _ := http.NewRequest("GET",
		"/oauth2/end_session?id_token_hint="+url.QueryEscape(idToken)+
			"&post_logout_redirect_uri=http://evil.example.com/steal",
		nil)
	rr4 := httptest.NewRecorder()
	h.EndSession(rr4, req4)
	loc4 := rr4.Header().Get("Location")
	if strings.Contains(loc4, "evil.example.com") {
		t.Errorf("case4: should not redirect to unregistered URI, got %q", loc4)
	}
	if !strings.Contains(loc4, "/auth/login") {
		t.Errorf("case4: expected fallback to login, got %q", loc4)
	}

	// ── Case 5: post_logout_redirect_uri without id_token_hint → ignored ────
	req5, _ := http.NewRequest("GET",
		"/oauth2/end_session?post_logout_redirect_uri=http://localhost/post-logout",
		nil)
	rr5 := httptest.NewRecorder()
	h.EndSession(rr5, req5)
	loc5 := rr5.Header().Get("Location")
	if strings.Contains(loc5, "post-logout") {
		t.Errorf("case5: redirect without hint should be ignored, got %q", loc5)
	}
}

// buildIDToken mints a signed ID token for use as an id_token_hint in tests.
func buildIDToken(t *testing.T, ks *auth.KeyStore, userID, clientID string) string {
	t.Helper()
	idClaims := jwt.MapClaims{
		"iss": "http://localhost",
		"sub": userID,
		"aud": clientID,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, idClaims)
	tok.Header["kid"] = ks.PrimaryKID()
	signed, err := tok.SignedString(ks.PrimaryKey())
	if err != nil {
		t.Fatalf("sign id token: %v", err)
	}
	return signed
}

package oidc

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/user"
)

type mockUserReader struct {
	user *user.User
}

func (m *mockUserReader) Create(ctx context.Context, email, passwordHash, locale string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserReader) CreateWithRoles(ctx context.Context, email, passwordHash, locale string, roles []string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserReader) FindByID(ctx context.Context, id string) (*user.User, error) {
	return m.user, nil
}
func (m *mockUserReader) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	return m.user, nil
}
func (m *mockUserReader) List(ctx context.Context, p user.ListParams) (user.ListResult, error) {
	return user.ListResult{}, nil
}
func (m *mockUserReader) SetRoles(ctx context.Context, userID string, roles []string) error {
	return nil
}
func (m *mockUserReader) SetActive(ctx context.Context, userID string, active bool) error {
	return nil
}

func (m *mockUserReader) BulkSetActive(_ context.Context, userIDs []string, active bool) error {
	return nil
}
func (m *mockUserReader) UpdateProfile(ctx context.Context, id, fn, ln string) (*user.User, error) {
	return nil, nil
}
func (m *mockUserReader) GetPermissions(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}
func (m *mockUserReader) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	return nil
}
func (m *mockUserReader) AssignRoleByName(ctx context.Context, userID, roleName string) error {
	return nil
}
func (m *mockUserReader) UpdateAvatar(ctx context.Context, id, key string, size int) (*user.User, error) {
	return nil, nil
}

func TestVerifyPKCE(t *testing.T) {
	tests := []struct {
		name      string
		challenge string
		method    string
		verifier  string
		wantErr   bool
	}{
		{
			name:      "S256 match — RFC 7636 example",
			challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			method:    "S256",
			verifier:  "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			wantErr:   false,
		},
		{
			// 43 chars, valid charset, but wrong hash
			name:      "S256 mismatch — correct length wrong hash",
			challenge: "E9Melhoa2OwvFrGMTJguCHaoeK1t8URWbuGJSstw-cM",
			method:    "S256",
			verifier:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			wantErr:   true,
		},
		{
			name:      "plain rejected — unsupported method",
			challenge: "my_verifier",
			method:    "plain",
			verifier:  "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			wantErr:   true,
		},
		{
			name:      "too short — below 43 chars",
			challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			method:    "S256",
			verifier:  "short",
			wantErr:   true,
		},
		{
			name:      "too long — above 128 chars",
			challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			method:    "S256",
			verifier:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			wantErr:   true,
		},
		{
			name:      "invalid charset — contains space",
			challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			method:    "S256",
			verifier:  "dBjftJeZ4CVP mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			wantErr:   true,
		},
		{
			name:      "invalid charset — contains equals sign",
			challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			method:    "S256",
			verifier:  "dBjftJeZ4CVP=mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyPKCE(tt.challenge, tt.method, tt.verifier)
			if (err != nil) != tt.wantErr {
				t.Errorf("verifyPKCE() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuthCodeStore(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	store := NewAuthCodeStore(rdb)
	session := &AuthCodeSession{
		Code:        "test-code-123",
		UserID:      "user-1",
		ClientID:    "client-1",
		RedirectURI: "http://localhost:3000/callback",
		Scopes:      []string{"openid", "profile"},
		AuthTime:    time.Now(),
	}

	ctx := context.Background()
	if err := store.Save(ctx, session); err != nil {
		t.Fatalf("save code failed: %v", err)
	}

	retrieved, err := store.GetAndConsume(ctx, "test-code-123")
	if err != nil {
		t.Fatalf("get code failed: %v", err)
	}

	if retrieved.UserID != "user-1" || retrieved.ClientID != "client-1" {
		t.Errorf("retrieved mismatch: %+v", retrieved)
	}

	// Verify single-use constraint (consumed on get)
	_, err = store.GetAndConsume(ctx, "test-code-123")
	if err != ErrAuthCodeNotFound {
		t.Errorf("expected ErrAuthCodeNotFound on second fetch, got %v", err)
	}
}

func TestExchangeCodeWithPKCE(t *testing.T) {
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

	userSvc := user.NewService(&mockUserReader{
		user: &user.User{
			ID:        "u123",
			Email:     "user@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Locale:    "en",
			IsActive:  true,
		},
	})

	codeStore := NewAuthCodeStore(rdb)
	refreshStore := NewRefreshTokenStore(rdb)
	svc := NewService(nil, codeStore, userSvc, ks, "https://issuer.example.com", refreshStore)

	session := &AuthCodeSession{
		Code:                "code-1",
		UserID:              "u123",
		ClientID:            "client-pkce",
		RedirectURI:         "http://localhost/callback",
		Scopes:              []string{"openid", "profile", "email", "offline_access"},
		CodeChallenge:       "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		CodeChallengeMethod: "S256",
		AuthTime:            time.Now(),
	}
	if err := codeStore.Save(context.Background(), session); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Exchange code successfully using PKCE verifier
	resp, err := svc.ExchangeCode(context.Background(), "code-1", "client-pkce", "", "http://localhost/callback", "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if err != nil {
		t.Fatalf("exchange failed: %v", err)
	}

	if resp.TokenType != "Bearer" {
		t.Errorf("token type = %q, want Bearer", resp.TokenType)
	}

	if resp.IDToken == "" {
		t.Errorf("expected ID token to be present")
	}

	// Verify ID token claims
	token, err := jwt.Parse(resp.IDToken, func(t *jwt.Token) (any, error) {
		pub, _, _ := ks.PublicKey(ks.PrimaryKID())
		return pub, nil
	})
	if err != nil {
		t.Fatalf("id token parse failed: %v", err)
	}

	claims := token.Claims.(jwt.MapClaims)
	if claims["sub"] != "u123" {
		t.Errorf("subject claim = %v, want u123", claims["sub"])
	}
	if claims["name"] != "Alice Smith" {
		t.Errorf("name claim = %v, want Alice Smith", claims["name"])
	}
	if claims["email"] != "user@example.com" {
		t.Errorf("email claim = %v, want user@example.com", claims["email"])
	}

	// ExchangeCode must issue a refresh token when the store is configured
	if resp.RefreshToken == "" {
		t.Errorf("expected refresh_token to be non-empty")
	}
}

// TestExchangeCodeWithES256KeyStore verifies the full authorization-code
// exchange round-trips with a non-default (ES256) signing algorithm: the ID
// token must carry alg=ES256 and validate against the ECDSA public key.
func TestExchangeCodeWithES256KeyStore(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgES256)
	if err != nil {
		t.Fatalf("keystore load: %v", err)
	}

	userSvc := user.NewService(&mockUserReader{
		user: &user.User{
			ID:        "u123",
			Email:     "user@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Locale:    "en",
			IsActive:  true,
		},
	})

	codeStore := NewAuthCodeStore(rdb)
	svc := NewService(nil, codeStore, userSvc, ks, "https://issuer.example.com", nil)

	session := &AuthCodeSession{
		Code:                "code-es256",
		UserID:              "u123",
		ClientID:            "client-pkce",
		RedirectURI:         "http://localhost/callback",
		Scopes:              []string{"openid", "profile", "email"},
		CodeChallenge:       "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		CodeChallengeMethod: "S256",
		AuthTime:            time.Now(),
	}
	if err := codeStore.Save(context.Background(), session); err != nil {
		t.Fatalf("save: %v", err)
	}

	resp, err := svc.ExchangeCode(context.Background(), "code-es256", "client-pkce", "", "http://localhost/callback", "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if err != nil {
		t.Fatalf("exchange failed: %v", err)
	}
	if resp.IDToken == "" {
		t.Fatal("expected ID token to be present")
	}

	token, err := jwt.Parse(resp.IDToken, func(tok *jwt.Token) (any, error) {
		pub, alg, _ := ks.PublicKey(ks.PrimaryKID())
		if tok.Method.Alg() != alg {
			t.Fatalf("id token alg=%q want=%q", tok.Method.Alg(), alg)
		}
		return pub, nil
	})
	if err != nil {
		t.Fatalf("id token parse failed: %v", err)
	}
	if token.Method.Alg() != auth.AlgES256 {
		t.Errorf("id token alg = %q, want ES256", token.Method.Alg())
	}
	if claims := token.Claims.(jwt.MapClaims); claims["sub"] != "u123" {
		t.Errorf("subject claim = %v, want u123", claims["sub"])
	}
}

func TestRefreshTokenStore(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	store := NewRefreshTokenStore(rdb)
	ctx := context.Background()

	sess := &OIDCRefreshSession{
		UserID:   "u1",
		ClientID: "c1",
		Scopes:   []string{"openid", "profile"},
		AuthTime: time.Now().Truncate(time.Second),
	}

	token, err := store.Save(ctx, sess)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(token) < 10 {
		t.Fatalf("token too short: %q", token)
	}

	got, err := store.GetAndConsume(ctx, token)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UserID != "u1" || got.ClientID != "c1" {
		t.Errorf("got %+v, want u1/c1", got)
	}

	// Single-use: second fetch must fail
	_, err = store.GetAndConsume(ctx, token)
	if err != ErrRefreshTokenNotFound {
		t.Errorf("expected ErrRefreshTokenNotFound on reuse, got %v", err)
	}
}

func TestExchangeRefreshToken(t *testing.T) {
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

	u := &user.User{ID: "u99", Email: "refresh@example.com", Locale: "en", IsActive: true}
	userSvc := user.NewService(&mockUserReader{user: u})
	refreshStore := NewRefreshTokenStore(rdb)
	svc := NewService(nil, nil, userSvc, ks, "https://issuer.example.com", refreshStore)

	ctx := context.Background()
	origScopes := []string{"openid", "profile", "email"}
	authTime := time.Now().Truncate(time.Second)

	origToken, err := refreshStore.Save(ctx, &OIDCRefreshSession{
		UserID:   u.ID,
		ClientID: "client-1",
		Scopes:   origScopes,
		AuthTime: authTime,
	})
	if err != nil {
		t.Fatalf("seed refresh token: %v", err)
	}

	// Successful exchange
	resp, err := svc.ExchangeRefreshToken(ctx, origToken, "client-1", "", nil)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access_token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected rotated refresh_token")
	}
	if resp.RefreshToken == origToken {
		t.Error("rotated token must differ from original")
	}
	if resp.Scope != "openid profile email" {
		t.Errorf("scope = %q, want openid profile email", resp.Scope)
	}

	// Original token must be consumed (single-use)
	_, err = svc.ExchangeRefreshToken(ctx, origToken, "client-1", "", nil)
	if err != ErrInvalidGrant {
		t.Errorf("expected ErrInvalidGrant on token reuse, got %v", err)
	}

	// Scope downscoping
	downscopedToken, _ := refreshStore.Save(ctx, &OIDCRefreshSession{
		UserID:   u.ID,
		ClientID: "client-1",
		Scopes:   origScopes,
		AuthTime: authTime,
	})
	resp2, err := svc.ExchangeRefreshToken(ctx, downscopedToken, "client-1", "", []string{"openid"})
	if err != nil {
		t.Fatalf("downscoped exchange: %v", err)
	}
	if resp2.Scope != "openid" {
		t.Errorf("downscoped scope = %q, want openid", resp2.Scope)
	}

	// Wrong client_id rejected
	wrongToken, _ := refreshStore.Save(ctx, &OIDCRefreshSession{
		UserID:   u.ID,
		ClientID: "client-1",
		Scopes:   origScopes,
		AuthTime: authTime,
	})
	_, err = svc.ExchangeRefreshToken(ctx, wrongToken, "other-client", "", nil)
	if err != ErrInvalidGrant {
		t.Errorf("expected ErrInvalidGrant for wrong client, got %v", err)
	}
}

func TestRevokeRefreshToken(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	refreshStore := NewRefreshTokenStore(rdb)
	svc := NewService(nil, nil, user.NewService(&mockUserReader{}), ks, "https://issuer.example.com", refreshStore)
	ctx := context.Background()

	sess := &OIDCRefreshSession{UserID: "u1", ClientID: "c1", Scopes: []string{"openid", "offline_access"}, AuthTime: time.Now()}
	token, _ := refreshStore.Save(ctx, sess)

	// Revoke
	svc.RevokeRefreshToken(ctx, token)

	// Token must be unusable after revocation
	_, err = refreshStore.GetAndConsume(ctx, token)
	if err != ErrRefreshTokenNotFound {
		t.Errorf("expected ErrRefreshTokenNotFound after revocation, got %v", err)
	}
}

func TestHasScope(t *testing.T) {
	tests := []struct {
		scopes []string
		target string
		want   bool
	}{
		{[]string{"openid", "offline_access"}, "offline_access", true},
		{[]string{"openid", "profile"}, "offline_access", false},
		{[]string{}, "offline_access", false},
		{[]string{"offline_access"}, "openid", false},
	}
	for _, tt := range tests {
		if got := hasScope(tt.scopes, tt.target); got != tt.want {
			t.Errorf("hasScope(%v, %q) = %v, want %v", tt.scopes, tt.target, got, tt.want)
		}
	}
}

// TestExchangeCode_InactiveUser verifies that a deactivated user cannot exchange
// an authorization code for tokens, even with a valid code and PKCE verifier.
func TestExchangeCode_InactiveUser(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	inactive := &user.User{ID: "u-inactive", Email: "inactive@example.com", Locale: "en", IsActive: false}
	codeStore := NewAuthCodeStore(rdb)
	svc := NewService(nil, codeStore, user.NewService(&mockUserReader{user: inactive}), ks, "https://issuer.example.com", nil)

	session := &AuthCodeSession{
		Code: "code-inactive", UserID: inactive.ID, ClientID: "c1",
		RedirectURI:         "http://localhost/cb",
		Scopes:              []string{"openid"},
		CodeChallenge:       "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		CodeChallengeMethod: "S256",
		AuthTime:            time.Now(),
	}
	codeStore.Save(context.Background(), session)

	_, err := svc.ExchangeCode(context.Background(), "code-inactive", "c1", "", "http://localhost/cb", "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if err != ErrInvalidGrant {
		t.Errorf("expected ErrInvalidGrant for inactive user on code exchange, got %v", err)
	}
}

// TestExchangeRefreshToken_InactiveUser verifies that a deactivated user cannot
// obtain new tokens via a previously issued OIDC refresh token.
func TestExchangeRefreshToken_InactiveUser(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	inactive := &user.User{ID: "u-inactive-rt", Email: "rt@example.com", Locale: "en", IsActive: false}
	refreshStore := NewRefreshTokenStore(rdb)
	svc := NewService(nil, nil, user.NewService(&mockUserReader{user: inactive}), ks, "https://issuer.example.com", refreshStore)

	rt, _ := refreshStore.Save(context.Background(), &OIDCRefreshSession{
		UserID:   inactive.ID,
		ClientID: "c1",
		Scopes:   []string{"openid", "offline_access"},
		AuthTime: time.Now(),
	})

	_, err := svc.ExchangeRefreshToken(context.Background(), rt, "c1", "", nil)
	if err != ErrInvalidGrant {
		t.Errorf("expected ErrInvalidGrant for inactive user on refresh exchange, got %v", err)
	}
}

func TestExchangeCode_NoRefreshWithoutOfflineAccess(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, _ := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	u := &user.User{ID: "u1", Email: "u@example.com", Locale: "en", IsActive: true}
	codeStore := NewAuthCodeStore(rdb)
	refreshStore := NewRefreshTokenStore(rdb)
	svc := NewService(nil, codeStore, user.NewService(&mockUserReader{user: u}), ks, "https://issuer.example.com", refreshStore)

	// Scopes without offline_access → no refresh token
	session := &AuthCodeSession{
		Code: "code-no-offline", UserID: u.ID, ClientID: "c1",
		RedirectURI: "http://localhost/cb", Scopes: []string{"openid", "profile"},
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", CodeChallengeMethod: "S256",
		AuthTime: time.Now(),
	}
	codeStore.Save(context.Background(), session)

	resp, err := svc.ExchangeCode(context.Background(), "code-no-offline", "c1", "", "http://localhost/cb", "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if resp.RefreshToken != "" {
		t.Errorf("expected no refresh_token without offline_access, got %q", resp.RefreshToken)
	}
}

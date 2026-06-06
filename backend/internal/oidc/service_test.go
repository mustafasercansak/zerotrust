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
			name:      "plain rejected",
			challenge: "my_verifier",
			method:    "plain",
			verifier:  "my_verifier",
			wantErr:   true,
		},
		{
			name:      "S256 match",
			challenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
			method:    "S256",
			verifier:  "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
			wantErr:   false,
		},
		{
			name:      "S256 mismatch",
			challenge: "E9Melhoa2OwvFrGMTJguCHaoeK1t8URWbuGJSstw-cM",
			method:    "S256",
			verifier:  "wrong_verifier",
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

	ks, err := auth.LoadOrGenerateKeyStore("", "")
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
		},
	})

	codeStore := NewAuthCodeStore(rdb)
	svc := NewService(nil, codeStore, userSvc, ks, "https://issuer.example.com")

	session := &AuthCodeSession{
		Code:                "code-1",
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
		pub, _ := ks.PublicKey(ks.PrimaryKID())
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
}

package auth_test

import (
	"testing"
	"time"

	"github.com/zerotrust/backend/internal/auth"
)

func newTestKeyStore(t *testing.T) *auth.KeyStore {
	t.Helper()
	ks, err := auth.LoadOrGenerateKeyStore("", "")
	if err != nil {
		t.Fatalf("key store: %v", err)
	}
	return ks
}

func TestGenerateAndValidateTokenPair(t *testing.T) {
	ks := newTestKeyStore(t)

	pair, err := auth.GenerateTokenPair(ks, "user-1", "test@example.com", "en",
		[]string{"admin"}, []string{"users:read"}, auth.AccessTTL)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("empty token in pair")
	}

	claims, err := auth.ValidateAccessToken(ks, pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-1")
	}
	if claims.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "test@example.com")
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Errorf("Roles = %v, want [admin]", claims.Roles)
	}
	if len(claims.Permissions) != 1 || claims.Permissions[0] != "users:read" {
		t.Errorf("Permissions = %v, want [users:read]", claims.Permissions)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	ks := newTestKeyStore(t)

	pair, err := auth.GenerateTokenPair(ks, "u", "e@e.com", "en", nil, nil, -1*time.Second)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	_, err = auth.ValidateAccessToken(ks, pair.AccessToken)
	if err != auth.ErrExpiredToken {
		t.Errorf("expected ErrExpiredToken, got %v", err)
	}
}

func TestTokenFromUnknownKeyRejected(t *testing.T) {
	ks1 := newTestKeyStore(t)
	ks2 := newTestKeyStore(t) // different ephemeral key

	pair, err := auth.GenerateTokenPair(ks1, "u", "e@e.com", "en", nil, nil, auth.AccessTTL)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	_, err = auth.ValidateAccessToken(ks2, pair.AccessToken)
	if err == nil {
		t.Fatal("expected error for unknown kid, got nil")
	}
}

func TestServiceToken(t *testing.T) {
	ks := newTestKeyStore(t)

	resp, err := auth.GenerateServiceToken(ks, "client-1", "ci-bot",
		[]string{"users:read", "service_accounts:read"}, 5*time.Minute)
	if err != nil {
		t.Fatalf("generate service token: %v", err)
	}

	claims, err := auth.ValidateAccessToken(ks, resp.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.ClientID != "client-1" {
		t.Errorf("ClientID = %q, want %q", claims.ClientID, "client-1")
	}
	if claims.SubType != auth.SubTypeService {
		t.Errorf("SubType = %q, want %q", claims.SubType, auth.SubTypeService)
	}
	if !claims.HasPermission("users", "read") {
		t.Error("HasPermission(users, read) = false, want true")
	}
	if claims.HasPermission("users", "write") {
		t.Error("HasPermission(users, write) = true, want false")
	}
}

func TestRefreshTokenIsOpaque(t *testing.T) {
	ks := newTestKeyStore(t)
	pair, _ := auth.GenerateTokenPair(ks, "u", "e@e.com", "en", nil, nil, auth.AccessTTL)

	// Refresh token must not be a valid JWT.
	_, err := auth.ValidateAccessToken(ks, pair.RefreshToken)
	if err == nil {
		t.Error("refresh token validated as JWT — it should be opaque")
	}
}

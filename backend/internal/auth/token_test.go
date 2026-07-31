package auth_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zerotrust/backend/internal/auth"
)

func newTestKeyStore(t *testing.T) *auth.KeyStore {
	t.Helper()
	ks, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
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
		[]string{"users:read", "service_accounts:read"}, 5*time.Minute, "")
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

func TestValidateAccessTokenFailures(t *testing.T) {
	ks := newTestKeyStore(t)

	// 1. Signed with HMAC (wrong algorithm)
	tokenHMAC := jwt.NewWithClaims(jwt.SigningMethodHS256, &auth.Claims{})
	tokenHMAC.Header["kid"] = ks.PrimaryKID()
	tokenStr, _ := tokenHMAC.SignedString([]byte("secret-key"))
	_, err := auth.ValidateAccessToken(ks, tokenStr)
	if err != auth.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}

	// 2. Missing kid
	tokenNoKid := jwt.NewWithClaims(jwt.SigningMethodEdDSA, &auth.Claims{})
	tokenStr, _ = tokenNoKid.SignedString(ks.PrimaryKey())
	_, err = auth.ValidateAccessToken(ks, tokenStr)
	if err != auth.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}

	// 3. Invalid kid type
	tokenBadKid := jwt.NewWithClaims(jwt.SigningMethodEdDSA, &auth.Claims{})
	tokenBadKid.Header["kid"] = 12345 // non-string kid
	tokenStr, _ = tokenBadKid.SignedString(ks.PrimaryKey())
	_, err = auth.ValidateAccessToken(ks, tokenStr)
	if err != auth.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestTokenRoundTripAllAlgorithms(t *testing.T) {
	for _, alg := range []string{auth.AlgEdDSA, auth.AlgES256, auth.AlgRS256} {
		t.Run(alg, func(t *testing.T) {
			ks, err := auth.LoadOrGenerateKeyStore("", "", alg)
			if err != nil {
				t.Fatalf("key store: %v", err)
			}

			pair, err := auth.GenerateTokenPair(ks, "user-1", "test@example.com", "en",
				[]string{"admin"}, []string{"users:read"}, auth.AccessTTL)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}

			claims, err := auth.ValidateAccessToken(ks, pair.AccessToken)
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if claims.UserID != "user-1" {
				t.Errorf("UserID = %q, want %q", claims.UserID, "user-1")
			}

			// The JWT header must advertise the configured algorithm.
			parsed, _, err := jwt.NewParser().ParseUnverified(pair.AccessToken, &auth.Claims{})
			if err != nil {
				t.Fatalf("parse unverified: %v", err)
			}
			if parsed.Method.Alg() != alg {
				t.Errorf("token alg = %q, want %q", parsed.Method.Alg(), alg)
			}
		})
	}
}

func TestAlgorithmConfusionRejected(t *testing.T) {
	// Keystore with an ES256 (ECDSA P-256) primary key.
	ks, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgES256)
	if err != nil {
		t.Fatalf("key store: %v", err)
	}

	// Craft a token whose header claims EdDSA but whose kid points at the
	// ES256 key. Validation must reject it before any signature check.
	edKey, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
	if err != nil {
		t.Fatalf("ed key store: %v", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, &auth.Claims{})
	token.Header["kid"] = ks.PrimaryKID()
	tokenStr, err := token.SignedString(edKey.PrimaryKey())
	if err != nil {
		t.Fatalf("sign crafted token: %v", err)
	}

	if _, err := auth.ValidateAccessToken(ks, tokenStr); err != auth.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for alg-confused token, got %v", err)
	}
}

func TestMixedAlgorithmRotation(t *testing.T) {
	// Primary ES256 with an EdDSA secondary: tokens signed by either key must
	// validate, each pinned to its own algorithm.
	tmp := t.TempDir()

	writeKey := func(name string, key any) string {
		t.Helper()
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatalf("marshal key: %v", err)
		}
		path := filepath.Join(tmp, name)
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
		if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}
		return path
	}

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ec key: %v", err)
	}
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed key: %v", err)
	}

	ks, err := auth.LoadOrGenerateKeyStore(writeKey("primary.pem", ecKey), writeKey("secondary.pem", edKey), auth.AlgES256)
	if err != nil {
		t.Fatalf("load keystore: %v", err)
	}
	if ks.PrimaryAlg() != auth.AlgES256 {
		t.Fatalf("PrimaryAlg=%q want=ES256", ks.PrimaryAlg())
	}

	// Token signed by the ES256 primary validates.
	pair, err := auth.GenerateTokenPair(ks, "u", "e@e.com", "en", nil, nil, auth.AccessTTL)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := auth.ValidateAccessToken(ks, pair.AccessToken); err != nil {
		t.Fatalf("validate primary token: %v", err)
	}

	// Token signed by the EdDSA secondary (as during rotation) validates too.
	secondary := jwt.NewWithClaims(jwt.SigningMethodEdDSA, &auth.Claims{
		UserID:   "u",
		TokenUse: auth.TokenUseSession,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
	})
	secondary.Header["kid"] = keyIDOf(t, ks, edKey)
	secondaryStr, err := secondary.SignedString(edKey)
	if err != nil {
		t.Fatalf("sign secondary token: %v", err)
	}
	if _, err := auth.ValidateAccessToken(ks, secondaryStr); err != nil {
		t.Fatalf("validate secondary token: %v", err)
	}
}

// keyIDOf finds the kid under which ks stored the given key's public part.
func keyIDOf(t *testing.T, ks *auth.KeyStore, key crypto.Signer) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	h := sha256.Sum256(der)
	kid := hex.EncodeToString(h[:8])
	if _, _, ok := ks.PublicKey(kid); !ok {
		t.Fatalf("kid %q not found in keystore", kid)
	}
	return kid
}

// TestValidateAccessTokenRejectsMissingTokenUse pins the audience-confusion
// guard (#81): a well-formed token signed by a trusted key but lacking the
// first-party token_use marker (e.g. an OIDC client access token) is rejected
// by the internal validator yet still accepted by the OIDC-facing one.
func TestValidateAccessTokenRejectsMissingTokenUse(t *testing.T) {
	ks := newTestKeyStore(t)

	mint := func(claims *auth.Claims) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		tok.Header["kid"] = ks.PrimaryKID()
		s, err := tok.SignedString(ks.PrimaryKey())
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		return s
	}

	oidcStyle := mint(&auth.Claims{
		UserID:  "u1",
		SubType: auth.SubTypeUser,
		Roles:   []string{"admin"},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "https://issuer.example.com",
			Subject:   "u1",
			Audience:  jwt.ClaimStrings{"some-external-client"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})

	if _, err := auth.ValidateAccessToken(ks, oidcStyle); err != auth.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for token without token_use, got %v", err)
	}
	claims, err := auth.ValidateOIDCAccessToken(ks, oidcStyle)
	if err != nil {
		t.Fatalf("OIDC validator must accept the token: %v", err)
	}
	if claims.UserID != "u1" {
		t.Errorf("UserID = %q, want u1", claims.UserID)
	}

	// Unknown token_use values are rejected too.
	oddUse := mint(&auth.Claims{
		UserID:   "u1",
		SubType:  auth.SubTypeUser,
		TokenUse: "made-up",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	if _, err := auth.ValidateAccessToken(ks, oddUse); err != auth.ErrInvalidToken {
		t.Errorf("expected ErrInvalidToken for unknown token_use, got %v", err)
	}
}

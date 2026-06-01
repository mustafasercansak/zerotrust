package auth_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zerotrust/backend/internal/auth"
)

func TestDPoPValidationSuccess(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	proof, err := auth.GenerateDPoPProofForTest(priv, "POST", "/api/v1/auth/token")
	if err != nil {
		t.Fatalf("generate proof: %v", err)
	}

	jkt, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token")
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	expectedJkt, err := auth.CalculateJKT(map[string]any{
		"kty": "OKP",
		"crv": "Ed25519",
		"x":   base64.RawURLEncoding.EncodeToString(pub),
	})
	if err != nil {
		t.Fatalf("calculate expected jkt: %v", err)
	}

	if jkt != expectedJkt {
		t.Errorf("jkt mismatch: got %q, want %q", jkt, expectedJkt)
	}
}

func TestDPoPValidationFailures(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Helper to generate a token with custom header/claims modifiers
	generateCustomProof := func(headers map[string]any, claims *auth.DPoPClaims) string {
		token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
		for k, v := range headers {
			token.Header[k] = v
		}
		s, _ := token.SignedString(priv)
		return s
	}

	stdClaims := func() *auth.DPoPClaims {
		return &auth.DPoPClaims{
			Jti: "test-jti",
			Htm: "POST",
			Htu: "/api/v1/auth/token",
			Iat: time.Now().Unix(),
		}
	}

	stdJWK := map[string]any{
		"kty": "OKP",
		"crv": "Ed25519",
		"x":   base64.RawURLEncoding.EncodeToString(pub),
	}

	// 1. Empty token
	if _, err := auth.ValidateDPoPProof("", "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for empty token, got nil")
	}

	// 2. Missing JWK
	proof := generateCustomProof(map[string]any{"typ": "dpop+jwt"}, stdClaims())
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for missing JWK, got nil")
	}

	// 3. Invalid JWK structure (not a map)
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": "not-a-map"}, stdClaims())
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for invalid JWK shape, got nil")
	}

	// 4. Unsupported kty (e.g. RSA)
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": map[string]any{"kty": "RSA"}}, stdClaims())
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for unsupported kty, got nil")
	}

	// 5. OKP with invalid curve
	badCrvJWK := map[string]any{"kty": "OKP", "crv": "Ed448", "x": "123"}
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": badCrvJWK}, stdClaims())
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for unsupported OKP curve, got nil")
	}

	// 6. OKP missing x
	missingXJWK := map[string]any{"kty": "OKP", "crv": "Ed25519"}
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": missingXJWK}, stdClaims())
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for missing OKP x parameter, got nil")
	}

	// 7. OKP invalid base64 x
	badBase64JWK := map[string]any{"kty": "OKP", "crv": "Ed25519", "x": "invalid base64!"}
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": badBase64JWK}, stdClaims())
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for invalid base64 x, got nil")
	}

	// 8. EC invalid curve
	badECCrvJWK := map[string]any{"kty": "EC", "crv": "P-384", "x": "123", "y": "456"}
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": badECCrvJWK}, stdClaims())
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for unsupported EC curve, got nil")
	}

	// 9. EC missing x or y
	missingECParamJWK := map[string]any{"kty": "EC", "crv": "P-256", "x": "123"}
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": missingECParamJWK}, stdClaims())
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for missing EC y parameter, got nil")
	}

	// 10. EC invalid base64 x or y
	badECBase64JWK := map[string]any{"kty": "EC", "crv": "P-256", "x": "invalid!", "y": "invalid!"}
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": badECBase64JWK}, stdClaims())
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for invalid EC base64 params, got nil")
	}

	// 11. Valid EC JWK verification parsing path (verifying signature with EC key requires signing EC, but let's test parser validation logic first)
	ecPriv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ecPub := ecPriv.Public().(*ecdsa.PublicKey)
	ecJWK := map[string]any{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(ecPub.X.Bytes()),
		"y":   base64.RawURLEncoding.EncodeToString(ecPub.Y.Bytes()),
	}
	ecToken := jwt.NewWithClaims(jwt.SigningMethodES256, stdClaims())
	ecToken.Header["typ"] = "dpop+jwt"
	ecToken.Header["jwk"] = ecJWK
	ecProof, _ := ecToken.SignedString(ecPriv)
	_, _ = auth.ValidateDPoPProof(ecProof, "POST", "/api/v1/auth/token") // trigger EC validation branch

	// 12. Invalid typ header
	proof = generateCustomProof(map[string]any{"typ": "jwt", "jwk": stdJWK}, stdClaims())
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for invalid typ header, got nil")
	}

	// 13. Expired proof claim iat
	expiredClaims := stdClaims()
	expiredClaims.Iat = time.Now().Unix() - 130
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": stdJWK}, expiredClaims)
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for expired proof iat, got nil")
	}

	// 14. Future proof claim iat
	futureClaims := stdClaims()
	futureClaims.Iat = time.Now().Unix() + 130
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": stdJWK}, futureClaims)
	if _, err := auth.ValidateDPoPProof(proof, "POST", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for future proof iat, got nil")
	}

	// 15. Method mismatch
	proof = generateCustomProof(map[string]any{"typ": "dpop+jwt", "jwk": stdJWK}, stdClaims())
	if _, err = auth.ValidateDPoPProof(proof, "GET", "/api/v1/auth/token"); err == nil {
		t.Error("expected error for method mismatch, got nil")
	}

	// 16. URI mismatch
	if _, err = auth.ValidateDPoPProof(proof, "POST", "/api/v1/other"); err == nil {
		t.Error("expected error for URI mismatch, got nil")
	}
}

func TestValidateHTU(t *testing.T) {
	// Generate valid proof with full URL HTU
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	
	// Test full URL parsing
	proof, _ := auth.GenerateDPoPProofForTest(priv, "GET", "https://example.com/api/v1/users?foo=bar#baz")
	if _, err := auth.ValidateDPoPProof(proof, "GET", "/api/v1/users"); err != nil {
		t.Errorf("expected success with full URL validation, got %v", err)
	}

	// Test suffix validation fallback
	proof2, _ := auth.GenerateDPoPProofForTest(priv, "GET", "/api/v1/users")
	if _, err := auth.ValidateDPoPProof(proof2, "GET", "/api/v1/users"); err != nil {
		t.Errorf("expected success with relative path, got %v", err)
	}
}

func TestCalculateJKTError(t *testing.T) {
	_, err := auth.CalculateJKT(map[string]any{"kty": "RSA"})
	if err == nil {
		t.Error("expected CalculateJKT to fail for unsupported kty, got nil")
	}
}

func TestDPoPRequiredMiddleware(t *testing.T) {
	handler := auth.DPoPRequiredMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Missing header
	req1 := httptest.NewRequest("GET", "/", nil)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing header, got %d", rr1.Code)
	}
	if !strings.Contains(rr1.Body.String(), "invalid_dpop_proof") {
		t.Errorf("unexpected error response body: %q", rr1.Body.String())
	}

	// Present header
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("DPoP", "some-proof")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200 for present header, got %d", rr2.Code)
	}
}

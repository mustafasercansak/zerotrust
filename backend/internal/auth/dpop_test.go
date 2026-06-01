package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

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
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	proof, err := auth.GenerateDPoPProofForTest(priv, "POST", "/api/v1/auth/token")
	if err != nil {
		t.Fatalf("generate proof: %v", err)
	}

	// 1. Method mismatch
	_, err = auth.ValidateDPoPProof(proof, "GET", "/api/v1/auth/token")
	if err == nil {
		t.Error("expected error for method mismatch, got nil")
	}

	// 2. URI mismatch
	_, err = auth.ValidateDPoPProof(proof, "POST", "/api/v1/other")
	if err == nil {
		t.Error("expected error for URI mismatch, got nil")
	}
}

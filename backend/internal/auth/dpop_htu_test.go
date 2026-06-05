package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/zerotrust/backend/internal/auth"
)

func proofWithHTU(t *testing.T, method, htu string) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	p, err := auth.GenerateDPoPProofForTest(priv, method, htu)
	if err != nil {
		t.Fatalf("generate proof: %v", err)
	}
	return p
}

func TestValidateHTU_RejectsSuffixSpoof(t *testing.T) {
	// htu path is a superset that previously passed via HasSuffix matching.
	proof := proofWithHTU(t, "GET", "/evil/api/v1/users")
	if _, err := auth.ValidateDPoPProof(proof, "GET", "/api/v1/users"); err == nil {
		t.Fatal("expected suffix-spoofed htu to be rejected")
	}
}

func TestValidateHTU_HostBinding(t *testing.T) {
	auth.SetExpectedDPoPOrigin("https://api.example.com")
	defer auth.SetExpectedDPoPOrigin("")

	// Correct origin + path is accepted.
	good := proofWithHTU(t, "POST", "https://api.example.com/api/v1/auth/token")
	if _, err := auth.ValidateDPoPProof(good, "POST", "/api/v1/auth/token"); err != nil {
		t.Fatalf("expected matching origin to succeed, got %v", err)
	}

	// Wrong host is rejected even though the path matches.
	badHost := proofWithHTU(t, "POST", "https://evil.example/api/v1/auth/token")
	if _, err := auth.ValidateDPoPProof(badHost, "POST", "/api/v1/auth/token"); err == nil {
		t.Fatal("expected mismatched host to be rejected")
	}

	// A bare-path htu is rejected when host binding is enabled.
	barePath := proofWithHTU(t, "POST", "/api/v1/auth/token")
	if _, err := auth.ValidateDPoPProof(barePath, "POST", "/api/v1/auth/token"); err == nil {
		t.Fatal("expected bare-path htu to be rejected when host binding is enabled")
	}
}

func TestValidateHTU_NoBindingAllowsPathOnly(t *testing.T) {
	// Default (origin unset) keeps path-only validation for dev/local clients.
	auth.SetExpectedDPoPOrigin("")
	proof := proofWithHTU(t, "GET", "/api/v1/users")
	if _, err := auth.ValidateDPoPProof(proof, "GET", "/api/v1/users"); err != nil {
		t.Fatalf("expected path-only validation to succeed with no origin set, got %v", err)
	}
}

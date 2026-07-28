package auth

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateOpaqueToken_ReturnsHex32Bytes(t *testing.T) {
	tok, err := generateOpaqueToken()
	if err != nil {
		t.Fatalf("generateOpaqueToken returned error: %v", err)
	}
	if len(tok) != 64 {
		t.Fatalf("token length=%d want=64", len(tok))
	}
	for _, r := range tok {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("token contains non-hex character %q", r)
		}
	}
}

func TestGenerateServiceToken_WithDPoPConfirmationClaim(t *testing.T) {
	ks, err := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeyStore: %v", err)
	}

	resp, err := GenerateServiceToken(ks, "client-42", "robot", []string{"users:read"}, 2*time.Minute, "jkt-abc")
	if err != nil {
		t.Fatalf("GenerateServiceToken: %v", err)
	}

	claims, err := ValidateAccessToken(ks, resp.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.Confirmation == nil {
		t.Fatal("confirmation claim is nil")
	}
	if claims.Confirmation.JKT != "jkt-abc" {
		t.Fatalf("cnf.jkt=%q want=jkt-abc", claims.Confirmation.JKT)
	}
}

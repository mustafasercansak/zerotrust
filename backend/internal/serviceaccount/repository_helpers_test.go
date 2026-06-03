package serviceaccount

import (
	"strings"
	"testing"
)

func TestGenerateClientID(t *testing.T) {
	id, err := generateClientID()
	if err != nil {
		t.Fatalf("generateClientID error: %v", err)
	}
	if !strings.HasPrefix(id, "svc_") {
		t.Fatalf("clientID=%q want prefix svc_", id)
	}
	if len(id) != 28 { // "svc_" (4) + 24 hex chars (12 bytes * 2)
		t.Fatalf("clientID length=%d want=28", len(id))
	}
	id2, _ := generateClientID()
	if id == id2 {
		t.Fatal("expected unique client IDs across calls")
	}
}

func TestGenerateSecret(t *testing.T) {
	s, err := generateSecret()
	if err != nil {
		t.Fatalf("generateSecret error: %v", err)
	}
	if len(s) != 64 { // 32 bytes * 2 hex chars each
		t.Fatalf("secret length=%d want=64", len(s))
	}
	s2, _ := generateSecret()
	if s == s2 {
		t.Fatal("expected unique secrets across calls")
	}
}

package auth

import (
	"testing"
	"time"
)

// TestProgressiveLockout verifies the lockout escalation thresholds.
func TestProgressiveLockout(t *testing.T) {
	cases := []struct {
		attempts int64
		want     time.Duration
	}{
		{0, 0},
		{1, 0},
		{4, 0},
		{5, 1 * time.Minute},
		{7, 1 * time.Minute},
		{8, 5 * time.Minute},
		{10, 5 * time.Minute},
		{11, 30 * time.Minute},
		{100, 30 * time.Minute},
	}
	for _, c := range cases {
		if got := progressiveLockout(c.attempts); got != c.want {
			t.Errorf("progressiveLockout(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}

// TestHashTokenDeterministic ensures the same input always produces the same hash.
func TestHashTokenDeterministic(t *testing.T) {
	h1 := hashToken("my-token")
	h2 := hashToken("my-token")
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q != %q", h1, h2)
	}
}

// TestHashTokenUnique ensures different inputs produce different hashes.
func TestHashTokenUnique(t *testing.T) {
	if hashToken("token-a") == hashToken("token-b") {
		t.Error("different tokens produced the same hash")
	}
}

// TestHashTokenLength ensures the hash is a 64-char hex string (SHA-256).
func TestHashTokenLength(t *testing.T) {
	h := hashToken("anything")
	if len(h) != 64 {
		t.Errorf("hash length = %d, want 64", len(h))
	}
}

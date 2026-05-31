package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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

type logoutSessionStore struct {
	revokedHash string
}

func (s *logoutSessionStore) Create(ctx context.Context, userID, tokenHash, ip, userAgent string, deviceInfo map[string]string, expiresAt time.Time) error {
	return nil
}

func (s *logoutSessionStore) RevokeForDevice(ctx context.Context, userID, ip, userAgent string, deviceInfo map[string]string) error {
	return nil
}

func (s *logoutSessionStore) RotateSession(ctx context.Context, oldHash string, generate func(userID string, lastActiveAt, currentExpiresAt time.Time) (newHash, ip, ua string, deviceInfo map[string]string, expiresAt time.Time, err error)) error {
	return nil
}

func (s *logoutSessionStore) Revoke(ctx context.Context, hash string) error {
	s.revokedHash = hash
	return nil
}

func (s *logoutSessionStore) EvictExcessSessions(ctx context.Context, userID string, keep int) error {
	return nil
}

func (s *logoutSessionStore) CheckReuse(ctx context.Context, hash string) (string, error) {
	return "", nil
}

func (s *logoutSessionStore) RevokeAllForUser(ctx context.Context, userID string) error {
	return nil
}

func TestLogoutRevokesRefreshSessionAndBlocklistsAccessToken(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, err := LoadOrGenerateKeyStore("", "")
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}

	sessions := &logoutSessionStore{}
	svc := NewService(nil, sessions, &testServiceAccountStore{}, rdb, ks, nil, nil)
	pair, err := GenerateTokenPair(ks, "u1", "user@example.com", "en", nil, nil, time.Hour)
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}
	claims, err := ValidateAccessToken(ks, pair.AccessToken)
	if err != nil {
		t.Fatalf("validate access token: %v", err)
	}

	if err := svc.Logout(context.Background(), "raw-refresh", pair.AccessToken); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}

	if sessions.revokedHash != hashToken("raw-refresh") {
		t.Fatalf("revoked hash=%q want=%q", sessions.revokedHash, hashToken("raw-refresh"))
	}
	if !svc.IsRevoked(context.Background(), claims.ID) {
		t.Fatal("expected access token JTI to be blocklisted")
	}
}

func TestLogoutIgnoresInvalidAccessToken(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, err := LoadOrGenerateKeyStore("", "")
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}

	svc := NewService(nil, &logoutSessionStore{}, &testServiceAccountStore{}, rdb, ks, nil, nil)
	if err := svc.Logout(context.Background(), "", "not-a-jwt"); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
}

func TestAuthKeyHelpersDeterministic(t *testing.T) {
	if jtiBlocklistKey("abc") != "jti:blocked:abc" {
		t.Fatalf("unexpected jti key: %q", jtiBlocklistKey("abc"))
	}
	if mfaPendingKey("xyz") != "mfa:pending:xyz" {
		t.Fatalf("unexpected mfa pending key: %q", mfaPendingKey("xyz"))
	}
	if lockoutKey("u@example.com") == lockoutKey("other@example.com") {
		t.Fatal("lockout keys should differ for different emails")
	}
	if failKey("u@example.com") == failKey("other@example.com") {
		t.Fatal("fail keys should differ for different emails")
	}
}

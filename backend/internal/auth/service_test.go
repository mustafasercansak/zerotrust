package auth

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/geoip"
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

	ks, err := LoadOrGenerateKeyStore("", "", AlgEdDSA)
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

	ks, err := LoadOrGenerateKeyStore("", "", AlgEdDSA)
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

func TestAccountLockedError(t *testing.T) {
	err := &AccountLockedError{RetryAfter: time.Minute}
	if err.Error() != "account_locked" {
		t.Errorf("expected account_locked, got %s", err.Error())
	}
}

type testMFAChecker struct {
	valid bool
}

func (m *testMFAChecker) IsEnabled(ctx context.Context, userID string) bool      { return true }
func (m *testMFAChecker) Validate(ctx context.Context, userID, code string) bool { return m.valid }
func (m *testMFAChecker) ValidateStepUp(ctx context.Context, userID, code string) bool {
	return m.valid
}
func (m *testMFAChecker) Setup(ctx context.Context, userID, email, currentCode string) (string, string, []string, error) {
	return "", "", nil, nil
}
func (m *testMFAChecker) VerifyAndEnable(ctx context.Context, userID, code string) error { return nil }

func TestMFAChallenge(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	ks, err := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}

	mockUser := &user.User{
		ID:       "u1",
		Email:    "test@example.com",
		IsActive: true,
	}
	usersReader := &testUserReader{byID: map[string]*user.User{"u1": mockUser}}
	mfaChecker := &testMFAChecker{valid: true}

	svc := NewService(usersReader, &logoutSessionStore{}, &testServiceAccountStore{}, rdb, ks, mfaChecker, nil)

	// Setup pending token in redis
	pendingToken := "pending123"
	hash := hashToken(pendingToken)
	key := mfaPendingKey(hash)

	m := map[string]interface{}{
		"uid":         "u1",
		"ip":          "127.0.0.1",
		"ua":          "test-ua",
		"device_info": map[string]string{"os": "linux"},
	}
	raw, _ := json.Marshal(m)
	rdb.Set(context.Background(), key, raw, time.Minute)

	pair, err := svc.MFAChallenge(context.Background(), pendingToken, "123456")
	if err != nil {
		t.Fatalf("MFAChallenge failed: %v", err)
	}
	if pair == nil || pair.AccessToken == "" {
		t.Fatalf("expected valid token pair")
	}
}

func TestLockout(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	svc := NewService(nil, nil, nil, rdb, nil, nil, nil)
	email := "test@example.com"
	ctx := context.Background()

	// Initial
	if err := svc.checkLockout(ctx, email); err != nil {
		t.Fatalf("expected no lockout, got %v", err)
	}

	// 5 failed attempts
	for i := 0; i < 5; i++ {
		svc.recordFailedAttempt(ctx, email, "1.1.1.1")
	}

	err = svc.checkLockout(ctx, email)
	if err == nil {
		t.Fatalf("expected lockout error")
	}
	lockedErr, ok := err.(*AccountLockedError)
	if !ok {
		t.Fatalf("expected AccountLockedError")
	}
	if lockedErr.RetryAfter <= 0 {
		t.Fatalf("expected retry after > 0")
	}

	// Clear attempts
	svc.clearFailedAttempts(ctx, email)
	if err := svc.checkLockout(ctx, email); err != nil {
		t.Fatalf("expected no lockout after clear, got %v", err)
	}
}

func TestFormatLocation(t *testing.T) {
	if got := formatLocation(&geoip.Location{City: "Istanbul", Country: "Turkey"}); got != "Istanbul, Turkey" {
		t.Fatalf("city+country got=%q want=Istanbul, Turkey", got)
	}
	if got := formatLocation(&geoip.Location{Country: "Turkey"}); got != "Turkey" {
		t.Fatalf("country only got=%q want=Turkey", got)
	}
	if got := formatLocation(&geoip.Location{}); got != "Unknown" {
		t.Fatalf("empty location got=%q want=Unknown", got)
	}
}

func TestSessionTimeoutHelpers(t *testing.T) {
	svcDefault := &Service{}
	if got := svcDefault.sessionIdleTimeout(context.Background()); got != defaultSessionIdleTimeout {
		t.Fatalf("sessionIdleTimeout default=%v want=%v", got, defaultSessionIdleTimeout)
	}
	if got := svcDefault.adminSessionIdleTimeout(context.Background()); got != defaultAdminSessionIdleTimeout {
		t.Fatalf("adminSessionIdleTimeout default=%v want=%v", got, defaultAdminSessionIdleTimeout)
	}
	if got := svcDefault.sessionAbsoluteTimeout(context.Background()); got != defaultSessionAbsoluteTimeout {
		t.Fatalf("sessionAbsoluteTimeout default=%v want=%v", got, defaultSessionAbsoluteTimeout)
	}

	settings := &testSettings{vals: map[string]int{
		settingSessionIdleTimeoutSec:      120,
		settingSessionIdleTimeoutAdminSec: 90,
		settingSessionAbsoluteTimeoutSec:  3600,
	}}
	svcCustom := &Service{settings: settings}
	if got := svcCustom.sessionIdleTimeout(context.Background()); got != 120*time.Second {
		t.Fatalf("sessionIdleTimeout custom=%v want=120s", got)
	}
	if got := svcCustom.adminSessionIdleTimeout(context.Background()); got != 90*time.Second {
		t.Fatalf("adminSessionIdleTimeout custom=%v want=90s", got)
	}
	if got := svcCustom.sessionAbsoluteTimeout(context.Background()); got != 3600*time.Second {
		t.Fatalf("sessionAbsoluteTimeout custom=%v want=3600s", got)
	}

	settingsZero := &testSettings{vals: map[string]int{
		settingSessionIdleTimeoutSec:      0,
		settingSessionIdleTimeoutAdminSec: -1,
		settingSessionAbsoluteTimeoutSec:  0,
	}}
	svcZero := &Service{settings: settingsZero}
	if got := svcZero.sessionIdleTimeout(context.Background()); got != defaultSessionIdleTimeout {
		t.Fatalf("sessionIdleTimeout fallback=%v want=%v", got, defaultSessionIdleTimeout)
	}
	if got := svcZero.adminSessionIdleTimeout(context.Background()); got != defaultAdminSessionIdleTimeout {
		t.Fatalf("adminSessionIdleTimeout fallback=%v want=%v", got, defaultAdminSessionIdleTimeout)
	}
	if got := svcZero.sessionAbsoluteTimeout(context.Background()); got != defaultSessionAbsoluteTimeout {
		t.Fatalf("sessionAbsoluteTimeout fallback=%v want=%v", got, defaultSessionAbsoluteTimeout)
	}
}

type mockMailer struct {
	sentAlerts []struct {
		to        string
		alertType string
		ipAddress string
		location  string
		details   string
	}
}

func (m *mockMailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	return nil
}

func (m *mockMailer) SendSecurityAlert(ctx context.Context, to, alertType, ipAddress, location, details string) error {
	m.sentAlerts = append(m.sentAlerts, struct {
		to        string
		alertType string
		ipAddress string
		location  string
		details   string
	}{to, alertType, ipAddress, location, details})
	return nil
}

func TestLockoutEmailAlert(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	svc := NewService(nil, nil, nil, rdb, nil, nil, nil)
	m := &mockMailer{}
	g := geoip.NewService("")
	svc.ConfigureSecurityAnomalies(g, m)

	email := "lockout_alert@example.com"
	ctx := context.Background()

	// 5 failed attempts
	for i := 0; i < 5; i++ {
		svc.recordFailedAttempt(ctx, email, "1.1.1.1")
	}

	if len(m.sentAlerts) != 1 {
		t.Fatalf("expected 1 alert sent, got %d", len(m.sentAlerts))
	}
	alert := m.sentAlerts[0]
	if alert.to != email {
		t.Errorf("expected recipient %s, got %s", email, alert.to)
	}
	if alert.alertType != "account_lockout" {
		t.Errorf("expected alert type 'account_lockout', got %s", alert.alertType)
	}
	if alert.ipAddress != "1.1.1.1" {
		t.Errorf("expected IP '1.1.1.1', got %s", alert.ipAddress)
	}
	if alert.location != "Sydney, Australia" {
		t.Errorf("expected location 'Sydney, Australia', got %q", alert.location)
	}
}


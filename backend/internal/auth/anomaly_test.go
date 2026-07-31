package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/geoip"
)

type testAnomalySessionStore struct {
	sessions []map[string]any
}

func (s *testAnomalySessionStore) Create(ctx context.Context, userID, tokenHash, ip, userAgent string, deviceInfo map[string]string, expiresAt time.Time) error {
	return nil
}
func (s *testAnomalySessionStore) RevokeForDevice(ctx context.Context, userID, ip, userAgent string, deviceInfo map[string]string) error {
	return nil
}
func (s *testAnomalySessionStore) RotateSession(ctx context.Context, oldHash string, generate func(userID string, lastActiveAt, currentExpiresAt time.Time) (newHash, ip, ua string, deviceInfo map[string]string, expiresAt time.Time, err error)) error {
	return nil
}
func (s *testAnomalySessionStore) Revoke(ctx context.Context, hash string) error {
	return nil
}
func (s *testAnomalySessionStore) EvictExcessSessions(ctx context.Context, userID string, keep int) error {
	return nil
}
func (s *testAnomalySessionStore) CheckReuse(ctx context.Context, hash string) (string, error) {
	return "", nil
}
func (s *testAnomalySessionStore) RevokeAllForUser(ctx context.Context, userID string) error {
	return nil
}
func (s *testAnomalySessionStore) GetActiveSessions(ctx context.Context, userID string) ([]map[string]any, error) {
	return s.sessions, nil
}

type testAnomalyMailer struct {
	sentAlerts []string
}

func (m *testAnomalyMailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	return nil
}

func (m *testAnomalyMailer) SendSecurityAlert(ctx context.Context, to, alertType, ipAddress, location, details string) error {
	m.sentAlerts = append(m.sentAlerts, fmt.Sprintf("%s:%s:%s", to, alertType, ipAddress))
	return nil
}

type dummyUserReader struct {
	u *user.User
}

func (r *dummyUserReader) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	return r.u, nil
}
func (r *dummyUserReader) FindByID(ctx context.Context, id string) (*user.User, error) {
	return r.u, nil
}
func (r *dummyUserReader) CheckPassword(hash, password string) bool {
	return true
}
func (r *dummyUserReader) GetPermissions(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func TestHaversineDistance(t *testing.T) {
	// London (51.507, -0.127) to New York (40.7128, -74.006) is roughly 5570 km
	d := haversineDistance(51.507, -0.127, 40.7128, -74.006)
	if math.Abs(d-5570.0) > 50.0 {
		t.Errorf("expected London to New York distance ~5570km, got %.1f", d)
	}
}

func TestLogin_ImpossibleTravel(t *testing.T) {
	u := &user.User{ID: "user-1", Email: "user1@example.com", IsActive: true, NotifySecurityEmails: true}
	reader := &dummyUserReader{u: u}

	// Active session in London
	sessions := []map[string]any{
		{
			"ip_address":   "100.0.0.2", // United Kingdom, London
			"user_agent":   "Chrome",
			"created_at":   time.Now().Add(-30 * time.Minute),
			"last_used_at": time.Now().Add(-30 * time.Minute),
		},
	}

	store := &testAnomalySessionStore{sessions: sessions}
	ks, _ := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	svc := NewService(reader, store, &testServiceAccountStore{}, nil, ks, nil, nil)

	g := geoip.NewService("")
	m := &testAnomalyMailer{}
	svc.ConfigureSecurityAnomalies(g, m)

	// Logging in from Tokyo (100.0.0.1) in 30 minutes is physically impossible (distance ~9500 km)
	res, err := svc.Login(context.Background(), "user1@example.com", "pass", "100.0.0.1", "Chrome", nil)
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	if res.AnomalyType != "impossible_travel" {
		t.Errorf("expected impossible_travel anomaly, got %q", res.AnomalyType)
	}

	// Wait briefly for background goroutine to complete
	time.Sleep(20 * time.Millisecond)

	if len(m.sentAlerts) != 1 {
		t.Fatalf("expected 1 security alert email sent, got %d", len(m.sentAlerts))
	}

	expectedAlert := "user1@example.com:impossible_travel:100.0.0.1"
	if m.sentAlerts[0] != expectedAlert {
		t.Errorf("expected alert %q, got %q", expectedAlert, m.sentAlerts[0])
	}
}

func TestLogin_NewDevice(t *testing.T) {
	u := &user.User{ID: "user-1", Email: "user1@example.com", IsActive: true}
	reader := &dummyUserReader{u: u}

	// Active session in New York on Firefox
	sessions := []map[string]any{
		{
			"ip_address":   "100.0.0.3", // United States, New York
			"user_agent":   "Firefox",
			"created_at":   time.Now().Add(-5 * time.Hour),
			"last_used_at": time.Now().Add(-5 * time.Hour),
		},
	}

	store := &testAnomalySessionStore{sessions: sessions}
	ks, _ := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	svc := NewService(reader, store, &testServiceAccountStore{}, nil, ks, nil, nil)

	g := geoip.NewService("")
	m := &testAnomalyMailer{}
	svc.ConfigureSecurityAnomalies(g, m)

	// Login from New York (100.0.0.3) but with Chrome (new device context, no travel anomaly since coordinates match exactly)
	res, err := svc.Login(context.Background(), "user1@example.com", "pass", "100.0.0.3", "Chrome", nil)
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	if res.AnomalyType != "new_device" {
		t.Errorf("expected new_device anomaly, got %q", res.AnomalyType)
	}
}

func TestLogin_SendsNewLoginAlert_WhenNoAnomaly(t *testing.T) {
	u := &user.User{ID: "user-1", Email: "user1@example.com", IsActive: true, NotifySecurityEmails: true}
	reader := &dummyUserReader{u: u}
	store := &testAnomalySessionStore{}
	ks, _ := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	svc := NewService(reader, store, &testServiceAccountStore{}, nil, ks, nil, nil)

	// Use an empty geoip (no anomaly detection) and a mailer
	g := geoip.NewService("")
	m := &testAnomalyMailer{}
	svc.ConfigureSecurityAnomalies(g, m)

	_, err := svc.Login(context.Background(), "user1@example.com", "pass", "1.2.3.4", "Chrome", nil)
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	if len(m.sentAlerts) != 1 {
		t.Fatalf("expected 1 new_login alert, got %d: %v", len(m.sentAlerts), m.sentAlerts)
	}
	if m.sentAlerts[0] != "user1@example.com:new_login:1.2.3.4" {
		t.Errorf("unexpected alert: %s", m.sentAlerts[0])
	}
}

func TestLogin_NoNewLoginAlert_WhenAnomalyDetected(t *testing.T) {
	u := &user.User{ID: "user-1", Email: "user1@example.com", IsActive: true, NotifySecurityEmails: true}
	reader := &dummyUserReader{u: u}

	sessions := []map[string]any{
		{
			"ip_address":   "100.0.0.2", // London
			"user_agent":   "Chrome",
			"created_at":   time.Now().Add(-30 * time.Minute),
			"last_used_at": time.Now().Add(-30 * time.Minute),
		},
	}
	store := &testAnomalySessionStore{sessions: sessions}
	ks, _ := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	svc := NewService(reader, store, &testServiceAccountStore{}, nil, ks, nil, nil)

	g := geoip.NewService("")
	m := &testAnomalyMailer{}
	svc.ConfigureSecurityAnomalies(g, m)

	// Login from Tokyo — triggers impossible_travel
	res, err := svc.Login(context.Background(), "user1@example.com", "pass", "100.0.0.1", "Chrome", nil)
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	if res.AnomalyType != "impossible_travel" {
		t.Errorf("expected impossible_travel, got %q", res.AnomalyType)
	}
	// Exactly 1 email: the anomaly alert — NOT a duplicate new_login alert
	if len(m.sentAlerts) != 1 {
		t.Fatalf("expected 1 alert (anomaly only), got %d: %v", len(m.sentAlerts), m.sentAlerts)
	}
	if m.sentAlerts[0] != "user1@example.com:impossible_travel:100.0.0.1" {
		t.Errorf("unexpected alert: %s", m.sentAlerts[0])
	}
}

type testAnomalySettings struct {
	enabled string
	mfa     int
	block   int
}

func (s *testAnomalySettings) GetInt(_ context.Context, key string, defaultVal int) int {
	if key == "risk_threshold_mfa" {
		return s.mfa
	}
	if key == "risk_threshold_block" {
		return s.block
	}
	return defaultVal
}
func (s *testAnomalySettings) GetString(_ context.Context, _ string, defaultVal string) string {
	return defaultVal
}
func (s *testAnomalySettings) GetBool(_ context.Context, key string, defaultVal bool) bool {
	if key == "risk_based_auth_enabled" {
		return s.enabled == "true"
	}
	return defaultVal
}

func TestLogin_RiskBasedAuthBlock(t *testing.T) {
	u := &user.User{ID: "user-1", Email: "user1@example.com", IsActive: true}
	reader := &dummyUserReader{u: u}

	// Active session in London
	sessions := []map[string]any{
		{
			"ip_address":   "100.0.0.2", // United Kingdom, London
			"user_agent":   "Chrome",
			"created_at":   time.Now().Add(-30 * time.Minute),
			"last_used_at": time.Now().Add(-30 * time.Minute),
		},
	}

	store := &testAnomalySessionStore{sessions: sessions}
	ks, _ := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	setts := &testAnomalySettings{enabled: "true", mfa: 40, block: 80}
	svc := NewService(reader, store, &testServiceAccountStore{}, nil, ks, nil, setts)

	g := geoip.NewService("")
	svc.ConfigureSecurityAnomalies(g, nil)

	// Logging in from Tokyo (100.0.0.1) in 30 minutes triggers impossible travel (+80 risk score)
	_, err := svc.Login(context.Background(), "user1@example.com", "pass", "100.0.0.1", "Chrome", nil)
	if err == nil {
		t.Fatalf("expected login to be blocked, got nil error")
	}
	if !errors.Is(err, ErrHighRiskBlocked) {
		t.Errorf("expected ErrHighRiskBlocked, got %v", err)
	}
}

type testAnomalyMFAChecker struct {
	enabled bool
}

func (c *testAnomalyMFAChecker) IsEnabled(_ context.Context, _ string) bool   { return c.enabled }
func (c *testAnomalyMFAChecker) Validate(_ context.Context, _, _ string) bool { return true }
func (c *testAnomalyMFAChecker) ValidateStepUp(_ context.Context, _, _ string) bool {
	return true
}
func (c *testAnomalyMFAChecker) Setup(_ context.Context, _, _, _ string) (string, string, []string, error) {
	return "url", "secret", []string{"code1"}, nil
}
func (c *testAnomalyMFAChecker) VerifyAndEnable(_ context.Context, _, _ string) error { return nil }

func TestLogin_RiskBasedAuthAdaptiveMFA(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	u := &user.User{ID: "user-1", Email: "user1@example.com", IsActive: true}
	reader := &dummyUserReader{u: u}

	// Active session in New York on Firefox
	sessions := []map[string]any{
		{
			"ip_address":   "100.0.0.3", // United States, New York
			"user_agent":   "Firefox",
			"created_at":   time.Now().Add(-5 * time.Hour),
			"last_used_at": time.Now().Add(-5 * time.Hour),
		},
	}

	store := &testAnomalySessionStore{sessions: sessions}
	ks, _ := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	setts := &testAnomalySettings{enabled: "true", mfa: 20, block: 90}
	mfaChecker := &testAnomalyMFAChecker{enabled: false} // User does not have MFA enabled
	svc := NewService(reader, store, &testServiceAccountStore{}, rdb, ks, mfaChecker, setts)

	g := geoip.NewService("")
	svc.ConfigureSecurityAnomalies(g, nil)

	// Login from New York (100.0.0.3) but with Chrome triggers new device anomaly (+30 risk score)
	res, err := svc.Login(context.Background(), "user1@example.com", "pass", "100.0.0.3", "Chrome", nil)
	if err != nil {
		t.Fatalf("unexpected login error: %v", err)
	}

	// Risk score is 30, which is >= mfa threshold (20) and < block threshold (90).
	// Since user doesn't have MFA setup, they should be forced to set up TOTP.
	if !res.MFARequired {
		t.Errorf("expected MFA to be required due to risk score")
	}
	if res.MFASetupSecret != "secret" {
		t.Errorf("expected MFA setup force-trigger secret, got %q", res.MFASetupSecret)
	}
}

func TestLogin_RiskScoreIncludesRecentFailedAttemptsBeforeClearing(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	const email = "risk-fails@example.com"
	u := &user.User{ID: "user-1", Email: email, IsActive: true}
	reader := &dummyUserReader{u: u}
	store := &testAnomalySessionStore{}
	ks, _ := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	setts := &testAnomalySettings{enabled: "true", mfa: 20, block: 100}
	mfaChecker := &testAnomalyMFAChecker{enabled: true}
	svc := NewService(reader, store, &testServiceAccountStore{}, rdb, ks, mfaChecker, setts)

	svc.recordFailedAttempt(context.Background(), email, "1.1.1.1")
	svc.recordFailedAttempt(context.Background(), email, "1.1.1.1")

	res, err := svc.Login(context.Background(), email, "pass", "1.2.3.4", "Chrome", nil)
	if err != nil {
		t.Fatalf("unexpected login error: %v", err)
	}
	if res.RiskScore != 30 {
		t.Fatalf("expected risk score 30 from two recent failures, got %d", res.RiskScore)
	}
	if res.AnomalyType != "multiple_failed_attempts" {
		t.Fatalf("expected multiple_failed_attempts reason, got %q", res.AnomalyType)
	}
	if !res.MFARequired {
		t.Fatal("expected adaptive MFA to be required")
	}
	if mr.Exists(failKey(email)) {
		t.Fatal("expected failed-attempt counter to be cleared after successful password check")
	}
}

func TestSuspiciousHoursWindow(t *testing.T) {
	tests := []struct {
		name       string
		hour       int
		start      int
		end        int
		expectedIn bool
	}{
		{name: "overnight in window", hour: 23, start: 23, end: 5, expectedIn: true},
		{name: "overnight after midnight", hour: 2, start: 23, end: 5, expectedIn: true},
		{name: "overnight out of window", hour: 12, start: 23, end: 5, expectedIn: false},
		{name: "same-day in window", hour: 14, start: 9, end: 18, expectedIn: true},
		{name: "same-day out of window", hour: 20, start: 9, end: 18, expectedIn: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isWithinSuspiciousHoursWindow(tc.hour, tc.start, tc.end); got != tc.expectedIn {
				t.Fatalf("isWithinSuspiciousHoursWindow(%d,%d,%d)=%v want=%v", tc.hour, tc.start, tc.end, got, tc.expectedIn)
			}
		})
	}
}

func TestDetectLoginAnomaly_KnownDeviceFingerprintNotFlaggedAsNew(t *testing.T) {
	u := &user.User{ID: "user-1", Email: "user1@example.com", IsActive: true}
	reader := &dummyUserReader{u: u}

	deviceInfo := map[string]any{
		"os":              "macos",
		"os_version":      "14.2",
		"browser":         "chrome",
		"browser_version": "120.1.10",
		"mobile":          "false",
		"architecture":    "arm64",
	}
	deviceJSON, _ := json.Marshal(deviceInfo)

	sessions := []map[string]any{
		{
			"ip_address":   "100.0.0.3", // New York
			"user_agent":   "Mozilla/5.0 Chrome/120.0.0",
			"device_info":  deviceJSON,
			"created_at":   time.Now().Add(-1 * time.Hour),
			"last_used_at": time.Now().Add(-30 * time.Minute),
		},
	}

	store := &testAnomalySessionStore{sessions: sessions}
	ks, _ := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	svc := NewService(reader, store, &testServiceAccountStore{}, nil, ks, nil, nil)
	svc.ConfigureSecurityAnomalies(geoip.NewService(""), nil)

	currentInfo := map[string]string{
		"os":              "macOS",
		"os_version":      "14.2",
		"browser":         "Chrome",
		"browser_version": "120.9.99",
		"mobile":          "false",
		"architecture":    "arm64",
	}

	has, kind, _ := svc.detectLoginAnomaly(context.Background(), u.ID, u.Email, "100.0.0.3", "Mozilla/5.0 Chrome/120.9.99", currentInfo)
	if has && kind == "new_device" {
		t.Fatalf("expected known fingerprint not to be flagged as new_device")
	}
}

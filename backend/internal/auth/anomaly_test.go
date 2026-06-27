package auth

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

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
	ks, _ := LoadOrGenerateKeyStore("", "")
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
	ks, _ := LoadOrGenerateKeyStore("", "")
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
	ks, _ := LoadOrGenerateKeyStore("", "")
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
	ks, _ := LoadOrGenerateKeyStore("", "")
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
	ks, _ := LoadOrGenerateKeyStore("", "")
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

func (c *testAnomalyMFAChecker) IsEnabled(_ context.Context, _ string) bool { return c.enabled }
func (c *testAnomalyMFAChecker) Validate(_ context.Context, _, _ string) bool { return true }
func (c *testAnomalyMFAChecker) Setup(_ context.Context, _, _, _ string) (string, string, []string, error) {
	return "url", "secret", []string{"code1"}, nil
}
func (c *testAnomalyMFAChecker) VerifyAndEnable(_ context.Context, _, _ string) error { return nil }

func TestLogin_RiskBasedAuthAdaptiveMFA(t *testing.T) {
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
	ks, _ := LoadOrGenerateKeyStore("", "")
	setts := &testAnomalySettings{enabled: "true", mfa: 20, block: 90}
	mfaChecker := &testAnomalyMFAChecker{enabled: false} // User does not have MFA enabled
	svc := NewService(reader, store, &testServiceAccountStore{}, nil, ks, mfaChecker, setts)
	
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

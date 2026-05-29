package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zerotrust/backend/internal/user"
)

type testUserReader struct {
	byID map[string]*user.User
}

type testSessionStoreSingleWinner struct {
	mu             sync.Mutex
	used           bool
	rotateUserID   string
	lastActiveAt   time.Time
	currentExpires time.Time
}

func (s *testSessionStoreSingleWinner) Create(ctx context.Context, userID, tokenHash, ip, userAgent string, deviceInfo map[string]string, expiresAt time.Time) error {
	return nil
}

func (s *testSessionStoreSingleWinner) RevokeForDevice(ctx context.Context, userID, ip, userAgent string, deviceInfo map[string]string) error {
	return nil
}

func (s *testSessionStoreSingleWinner) RotateSession(ctx context.Context, oldHash string, generate func(userID string, lastActiveAt, currentExpiresAt time.Time) (newHash, ip, ua string, deviceInfo map[string]string, expiresAt time.Time, err error)) error {
	s.mu.Lock()
	if s.used {
		s.mu.Unlock()
		return errors.New("already rotated")
	}
	s.used = true
	s.mu.Unlock()

	_, _, _, _, _, err := generate(s.rotateUserID, s.lastActiveAt, s.currentExpires)
	return err
}

func (s *testSessionStoreSingleWinner) Revoke(ctx context.Context, hash string) error {
	return nil
}

func (s *testSessionStoreSingleWinner) EvictExcessSessions(ctx context.Context, userID string, keep int) error {
	return nil
}

func (s *testSessionStoreSingleWinner) CheckReuse(ctx context.Context, hash string) (string, error) {
	return "", nil
}

func (s *testSessionStoreSingleWinner) RevokeAllForUser(ctx context.Context, userID string) error {
	return nil
}

func (r *testUserReader) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	return nil, user.ErrNotFound
}

func (r *testUserReader) FindByID(ctx context.Context, id string) (*user.User, error) {
	u, ok := r.byID[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}

func (r *testUserReader) CheckPassword(hash, password string) bool {
	return false
}

func (r *testUserReader) GetPermissions(ctx context.Context, userID string) ([]string, error) {
	return []string{"sessions:read"}, nil
}

type testSessionStore struct {
	rotateUserID     string
	rotateLastActive time.Time
	rotateExpiresAt  time.Time

	receivedNewExpiry time.Time
	rotateErr         error
}

func (s *testSessionStore) Create(ctx context.Context, userID, tokenHash, ip, userAgent string, deviceInfo map[string]string, expiresAt time.Time) error {
	return nil
}

func (s *testSessionStore) RevokeForDevice(ctx context.Context, userID, ip, userAgent string, deviceInfo map[string]string) error {
	return nil
}

func (s *testSessionStore) RotateSession(ctx context.Context, oldHash string, generate func(userID string, lastActiveAt, currentExpiresAt time.Time) (newHash, ip, ua string, deviceInfo map[string]string, expiresAt time.Time, err error)) error {
	if s.rotateErr != nil {
		return s.rotateErr
	}
	_, _, _, _, exp, err := generate(s.rotateUserID, s.rotateLastActive, s.rotateExpiresAt)
	if err != nil {
		return err
	}
	s.receivedNewExpiry = exp
	return nil
}

func (s *testSessionStore) Revoke(ctx context.Context, hash string) error {
	return nil
}

func (s *testSessionStore) EvictExcessSessions(ctx context.Context, userID string, keep int) error {
	return nil
}

func (s *testSessionStore) CheckReuse(ctx context.Context, hash string) (string, error) {
	return "", nil
}

func (s *testSessionStore) RevokeAllForUser(ctx context.Context, userID string) error {
	return nil
}

type testSettings struct {
	vals map[string]int
}

func (s *testSettings) GetInt(ctx context.Context, key string, defaultVal int) int {
	v, ok := s.vals[key]
	if !ok {
		return defaultVal
	}
	return v
}

func (s *testSettings) GetString(ctx context.Context, key string, defaultVal string) string {
	return defaultVal
}

func (s *testSettings) GetBool(ctx context.Context, key string, defaultVal bool) bool {
	return defaultVal
}


type testServiceAccountStore struct{}

func (s *testServiceAccountStore) FindByClientID(ctx context.Context, clientID string) (*ServiceAccountRecord, error) {
	return nil, errors.New("not used in refresh tests")
}

func (s *testServiceAccountStore) CheckSecret(hash, secret string) bool {
	return false
}

func newRefreshPolicyService(t *testing.T, u *user.User, store SessionStore, settings map[string]int) *Service {
	t.Helper()
	ks, err := LoadOrGenerateKeyStore("", "")
	if err != nil {
		t.Fatalf("keystore init failed: %v", err)
	}

	return NewService(
		&testUserReader{byID: map[string]*user.User{u.ID: u}},
		store,
		&testServiceAccountStore{},
		nil,
		ks,
		nil,
		&testSettings{vals: settings},
	)
}

func TestRefreshTokens_SucceedsWithinIdleWindow(t *testing.T) {
	now := time.Now()
	store := &testSessionStore{
		rotateUserID:     "u1",
		rotateLastActive: now.Add(-2 * time.Minute),
		rotateExpiresAt:  now.Add(2 * time.Hour),
	}
	svc := newRefreshPolicyService(t, &user.User{ID: "u1", Email: "u1@example.com", Locale: "en", Roles: []string{"viewer"}}, store, nil)

	pair, err := svc.RefreshTokens(context.Background(), "refresh-token", "127.0.0.1", "ua", map[string]string{"browser": "test"})
	if err != nil {
		t.Fatalf("RefreshTokens returned error: %v", err)
	}
	if pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("expected non-empty token pair, got %#v", pair)
	}
}

func TestRefreshTokens_RejectsWhenIdleTimeoutExceeded(t *testing.T) {
	now := time.Now()
	store := &testSessionStore{
		rotateUserID:     "u1",
		rotateLastActive: now.Add(-6 * time.Minute),
		rotateExpiresAt:  now.Add(2 * time.Hour),
	}
	svc := newRefreshPolicyService(t, &user.User{ID: "u1", Email: "u1@example.com", Locale: "en", Roles: []string{"viewer"}}, store, nil)

	_, err := svc.RefreshTokens(context.Background(), "refresh-token", "127.0.0.1", "ua", nil)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRefreshTokens_AdminUsesStricterIdleTimeout(t *testing.T) {
	now := time.Now()
	store := &testSessionStore{
		rotateUserID:     "admin1",
		rotateLastActive: now.Add(-4 * time.Minute),
		rotateExpiresAt:  now.Add(2 * time.Hour),
	}
	svc := newRefreshPolicyService(t, &user.User{ID: "admin1", Email: "admin@example.com", Locale: "en", Roles: []string{"admin"}}, store, nil)

	_, err := svc.RefreshTokens(context.Background(), "refresh-token", "127.0.0.1", "ua", nil)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for admin strict idle timeout, got %v", err)
	}
}

func TestRefreshTokens_RejectsWhenAbsoluteExpiryPassed(t *testing.T) {
	now := time.Now()
	store := &testSessionStore{
		rotateUserID:     "u1",
		rotateLastActive: now.Add(-1 * time.Minute),
		rotateExpiresAt:  now.Add(-1 * time.Second),
	}
	svc := newRefreshPolicyService(t, &user.User{ID: "u1", Email: "u1@example.com", Locale: "en", Roles: []string{"viewer"}}, store, nil)

	_, err := svc.RefreshTokens(context.Background(), "refresh-token", "127.0.0.1", "ua", nil)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for expired absolute session, got %v", err)
	}
}

func TestRefreshTokens_PreservesAbsoluteExpiryAcrossRotation(t *testing.T) {
	now := time.Now()
	abs := now.Add(3 * time.Hour)
	store := &testSessionStore{
		rotateUserID:     "u1",
		rotateLastActive: now.Add(-2 * time.Minute),
		rotateExpiresAt:  abs,
	}
	svc := newRefreshPolicyService(t, &user.User{ID: "u1", Email: "u1@example.com", Locale: "en", Roles: []string{"viewer"}}, store, nil)

	_, err := svc.RefreshTokens(context.Background(), "refresh-token", "127.0.0.1", "ua", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !store.receivedNewExpiry.Equal(abs) {
		t.Fatalf("expected rotated session to keep absolute expiry %v, got %v", abs, store.receivedNewExpiry)
	}
}

func TestRefreshTokens_ConcurrentOnlyOneSucceeds(t *testing.T) {
	now := time.Now()
	store := &testSessionStoreSingleWinner{
		rotateUserID:   "u1",
		lastActiveAt:   now.Add(-1 * time.Minute),
		currentExpires: now.Add(2 * time.Hour),
	}
	svc := newRefreshPolicyService(t, &user.User{ID: "u1", Email: "u1@example.com", Locale: "en", Roles: []string{"viewer"}}, store, nil)

	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := svc.RefreshTokens(context.Background(), "same-refresh-token", "127.0.0.1", "ua", nil)
			results <- err
		}()
	}

	err1 := <-results
	err2 := <-results

	successes := 0
	failures := 0
	for _, err := range []error{err1, err2} {
		if err == nil {
			successes++
			continue
		}
		if errors.Is(err, ErrInvalidToken) {
			failures++
			continue
		}
		t.Fatalf("unexpected error type: %v", err)
	}

	if successes != 1 || failures != 1 {
		t.Fatalf("expected exactly one success and one ErrInvalidToken, got successes=%d failures=%d (err1=%v err2=%v)", successes, failures, err1, err2)
	}
}

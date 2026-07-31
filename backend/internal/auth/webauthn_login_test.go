package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/user"
)

type fakeWebAuthnVerifier struct {
	hasCreds      bool
	beginOpts     json.RawMessage
	beginErr      error
	finishErr     error
	beginCalls    int
	finishCalls   int
	discoOpts     json.RawMessage
	discoBeginErr error
	discoUserID   string
	discoErr      error
}

func (f *fakeWebAuthnVerifier) HasCredentials(_ context.Context, _ string) bool { return f.hasCreds }
func (f *fakeWebAuthnVerifier) BeginLogin(_ context.Context, _, _, _ string) (json.RawMessage, error) {
	f.beginCalls++
	return f.beginOpts, f.beginErr
}
func (f *fakeWebAuthnVerifier) FinishLogin(_ context.Context, _, _, _ string, _ []byte) error {
	f.finishCalls++
	return f.finishErr
}
func (f *fakeWebAuthnVerifier) BeginDiscoverableLogin(_ context.Context) (json.RawMessage, error) {
	return f.discoOpts, f.discoBeginErr
}
func (f *fakeWebAuthnVerifier) FinishDiscoverableLogin(_ context.Context, _ string, _ []byte) (string, error) {
	return f.discoUserID, f.discoErr
}

type waLoginUserReader struct{ u *user.User }

func (r *waLoginUserReader) FindByEmail(_ context.Context, _ string) (*user.User, error) {
	if r.u == nil {
		return nil, user.ErrNotFound
	}
	return r.u, nil
}
func (r *waLoginUserReader) FindByID(_ context.Context, _ string) (*user.User, error) {
	if r.u == nil {
		return nil, user.ErrNotFound
	}
	return r.u, nil
}
func (r *waLoginUserReader) CheckPassword(_, _ string) bool { return true }
func (r *waLoginUserReader) GetPermissions(_ context.Context, _ string) ([]string, error) {
	return []string{"sessions:read"}, nil
}

func newWebAuthnTestService(t *testing.T, verifier WebAuthnVerifier) (*Service, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	ks, err := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}
	u := &user.User{ID: "u1", Email: "user@example.com", IsActive: true, PasswordHash: "x", Locale: "en"}
	svc := NewService(&waLoginUserReader{u: u}, &logoutSessionStore{}, &testServiceAccountStore{}, rdb, ks, nil, nil)
	svc.ConfigureWebAuthn(verifier)
	return svc, rdb
}

func TestLogin_FlagsWebAuthnAsSecondFactor(t *testing.T) {
	verifier := &fakeWebAuthnVerifier{hasCreds: true}
	svc, _ := newWebAuthnTestService(t, verifier)

	res, err := svc.Login(context.Background(), "user@example.com", "pw", "1.2.3.4", "ua", nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !res.MFARequired {
		t.Fatal("expected MFARequired when a passkey is registered")
	}
	if !res.WebAuthnEnabled {
		t.Fatal("expected WebAuthnEnabled=true")
	}
	if res.TOTPEnabled {
		t.Fatal("expected TOTPEnabled=false (no TOTP configured)")
	}
	if res.MFAPendingToken == "" {
		t.Fatal("expected a pending token")
	}
	if res.Pair != nil {
		t.Fatal("login must not complete before the second factor")
	}
}

func TestWebAuthnLoginBegin_ReturnsOptions(t *testing.T) {
	verifier := &fakeWebAuthnVerifier{hasCreds: true, beginOpts: json.RawMessage(`{"publicKey":{}}`)}
	svc, rdb := newWebAuthnTestService(t, verifier)

	// Seed a pending login record.
	token := "pending-abc"
	raw, _ := json.Marshal(map[string]any{"uid": "u1", "ip": "1.2.3.4", "ua": "ua"})
	rdb.Set(context.Background(), mfaPendingKey(hashToken(token)), raw, time.Minute)

	opts, err := svc.WebAuthnLoginBegin(context.Background(), token)
	if err != nil {
		t.Fatalf("WebAuthnLoginBegin: %v", err)
	}
	if string(opts) != `{"publicKey":{}}` {
		t.Fatalf("unexpected options: %s", opts)
	}
	if verifier.beginCalls != 1 {
		t.Fatalf("expected BeginLogin called once, got %d", verifier.beginCalls)
	}

	// Unknown token is rejected.
	if _, err := svc.WebAuthnLoginBegin(context.Background(), "nope"); err == nil {
		t.Fatal("expected error for unknown pending token")
	}
}

func TestWebAuthnLoginFinish_Success(t *testing.T) {
	verifier := &fakeWebAuthnVerifier{hasCreds: true}
	svc, rdb := newWebAuthnTestService(t, verifier)
	ctx := context.Background()

	token := "pending-xyz"
	key := mfaPendingKey(hashToken(token))
	raw, _ := json.Marshal(map[string]any{"uid": "u1", "ip": "1.2.3.4", "ua": "ua", "device_info": map[string]string{"os": "linux"}})
	rdb.Set(ctx, key, raw, time.Minute)

	pair, err := svc.WebAuthnLoginFinish(ctx, token, []byte(`{"id":"abc"}`))
	if err != nil {
		t.Fatalf("WebAuthnLoginFinish: %v", err)
	}
	if pair == nil || pair.AccessToken == "" {
		t.Fatal("expected a token pair")
	}
	if verifier.finishCalls != 1 {
		t.Fatalf("expected FinishLogin called once, got %d", verifier.finishCalls)
	}
	// The pending token is consumed on success.
	if err := rdb.Get(ctx, key).Err(); err == nil {
		t.Fatal("expected pending token to be deleted after success")
	}
}

// TestWebAuthnLoginFinish_InactiveUserRejected covers the case where the user
// is deactivated between the password step and the passkey assertion: the
// finish path must reject like its siblings (MFAChallenge, passwordless) (#97).
func TestWebAuthnLoginFinish_InactiveUserRejected(t *testing.T) {
	verifier := &fakeWebAuthnVerifier{hasCreds: true}
	svc, rdb := newWebAuthnTestService(t, verifier)
	ctx := context.Background()

	token := "pending-inactive"
	key := mfaPendingKey(hashToken(token))
	raw, _ := json.Marshal(map[string]any{"uid": "u1", "ip": "1.2.3.4", "ua": "ua"})
	rdb.Set(ctx, key, raw, time.Minute)

	// Deactivate the user after the password step created the pending login.
	svc.users.(*waLoginUserReader).u.IsActive = false

	if _, err := svc.WebAuthnLoginFinish(ctx, token, []byte(`{"id":"abc"}`)); err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for inactive user, got %v", err)
	}
}

func TestWebAuthnLoginFinish_VerificationFails(t *testing.T) {
	verifier := &fakeWebAuthnVerifier{hasCreds: true, finishErr: ErrInvalidToken}
	svc, rdb := newWebAuthnTestService(t, verifier)
	ctx := context.Background()

	token := "pending-bad"
	key := mfaPendingKey(hashToken(token))
	raw, _ := json.Marshal(map[string]any{"uid": "u1", "ip": "1.2.3.4", "ua": "ua"})
	rdb.Set(ctx, key, raw, time.Minute)

	if _, err := svc.WebAuthnLoginFinish(ctx, token, []byte(`{"id":"abc"}`)); err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	// On failure the pending token is preserved so the user can retry.
	if err := rdb.Get(ctx, key).Err(); err != nil {
		t.Fatalf("expected pending token to survive a failed assertion, got %v", err)
	}
}

func TestWebAuthnPasswordlessBegin_ReturnsOptions(t *testing.T) {
	verifier := &fakeWebAuthnVerifier{discoOpts: json.RawMessage(`{"publicKey":{},"ceremony_id":"c1"}`)}
	svc, _ := newWebAuthnTestService(t, verifier)

	opts, err := svc.WebAuthnPasswordlessBegin(context.Background())
	if err != nil {
		t.Fatalf("WebAuthnPasswordlessBegin: %v", err)
	}
	if string(opts) != `{"publicKey":{},"ceremony_id":"c1"}` {
		t.Fatalf("unexpected options: %s", opts)
	}
}

func TestWebAuthnPasswordlessBegin_VerifierError(t *testing.T) {
	verifier := &fakeWebAuthnVerifier{discoBeginErr: errors.New("boom")}
	svc, _ := newWebAuthnTestService(t, verifier)

	if _, err := svc.WebAuthnPasswordlessBegin(context.Background()); err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestWebAuthnPasswordlessFinish_Success(t *testing.T) {
	verifier := &fakeWebAuthnVerifier{discoUserID: "u1"}
	svc, _ := newWebAuthnTestService(t, verifier)

	pair, err := svc.WebAuthnPasswordlessFinish(context.Background(), "c1", []byte(`{"id":"abc"}`), "1.2.3.4", "ua", nil)
	if err != nil {
		t.Fatalf("WebAuthnPasswordlessFinish: %v", err)
	}
	if pair == nil || pair.AccessToken == "" {
		t.Fatal("expected a token pair")
	}
}

func TestWebAuthnPasswordlessFinish_AssertionFails(t *testing.T) {
	verifier := &fakeWebAuthnVerifier{discoErr: errors.New("bad assertion")}
	svc, _ := newWebAuthnTestService(t, verifier)

	if _, err := svc.WebAuthnPasswordlessFinish(context.Background(), "c1", []byte(`{"id":"abc"}`), "1.2.3.4", "ua", nil); err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestWebAuthnPasswordlessFinish_InactiveUser(t *testing.T) {
	verifier := &fakeWebAuthnVerifier{discoUserID: "u1"}
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ks, err := LoadOrGenerateKeyStore("", "", AlgEdDSA)
	if err != nil {
		t.Fatalf("keystore: %v", err)
	}
	inactive := &user.User{ID: "u1", Email: "user@example.com", IsActive: false, PasswordHash: "x", Locale: "en"}
	svc := NewService(&waLoginUserReader{u: inactive}, &logoutSessionStore{}, &testServiceAccountStore{}, rdb, ks, nil, nil)
	svc.ConfigureWebAuthn(verifier)

	if _, err := svc.WebAuthnPasswordlessFinish(context.Background(), "c1", []byte(`{"id":"abc"}`), "1.2.3.4", "ua", nil); err != ErrInvalidCredentials {
		t.Fatalf("expected ErrInvalidCredentials for inactive user, got %v", err)
	}
}

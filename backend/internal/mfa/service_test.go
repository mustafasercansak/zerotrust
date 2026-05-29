package mfa

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	appCrypto "github.com/zerotrust/backend/pkg/crypto"
)

// stubStore implements store for testing without a real database.
type stubStore struct {
	enabled      bool
	secretEnc    string // active encrypted secret
	pendingEnc   string // pending encrypted secret (written by UpsertPending)
	upsertErr    error
	enableErr    error
	enableCalled bool
}

func (s *stubStore) IsEnabled(_ context.Context, _ string) bool { return s.enabled }

func (s *stubStore) UpsertPending(_ context.Context, _, enc string) error {
	s.pendingEnc = enc
	return s.upsertErr
}

func (s *stubStore) Enable(_ context.Context, _ string) error {
	s.enableCalled = true
	return s.enableErr
}

func (s *stubStore) Delete(_ context.Context, _ string) error { return nil }

func (s *stubStore) SecretEnc(_ context.Context, _ string) (string, error) {
	if s.secretEnc == "" {
		return "", ErrNotFound
	}
	return s.secretEnc, nil
}

func (s *stubStore) PendingSecretEnc(_ context.Context, _ string) (string, error) {
	if s.pendingEnc == "" {
		return "", ErrNotFound
	}
	return s.pendingEnc, nil
}

// testKey returns a deterministic 32-byte AES key suitable for tests only.
func testKey() []byte { return make([]byte, 32) }

// encryptSecret encrypts a TOTP secret with the given key and returns its hex encoding.
func encryptSecret(t *testing.T, key []byte, secret string) string {
	t.Helper()
	enc, err := appCrypto.Encrypt(key, []byte(secret))
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	return hex.EncodeToString(enc)
}

// newTOTPKey generates a real TOTP key and returns its secret and a current valid code.
func newTOTPKey(t *testing.T) (secret, code string) {
	t.Helper()
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "test", AccountName: "test@example.com"})
	if err != nil {
		t.Fatalf("generate TOTP key: %v", err)
	}
	code, err = totp.GenerateCode(key.Secret(), time.Now())
	if err != nil {
		t.Fatalf("generate TOTP code: %v", err)
	}
	return key.Secret(), code
}
// --- Setup tests ---

// TestSetup_FirstTime verifies that Setup succeeds with an empty current_code
// when MFA is not yet enabled.
func TestSetup_FirstTime(t *testing.T) {
	stub := &stubStore{enabled: false}
	svc := &Service{repo: stub, encKey: testKey()}

	url, secret, err := svc.Setup(context.Background(), "user1", "user1@example.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" || secret == "" {
		t.Error("expected non-empty setup result")
	}
	if stub.pendingEnc == "" {
		t.Error("UpsertPending was not called")
	}
}
func TestSetup_MFAEnabled_EmptyCode(t *testing.T) {
	key := testKey()
	totpSecret, _ := newTOTPKey(t)
	stub := &stubStore{enabled: true, secretEnc: encryptSecret(t, key, totpSecret)}
	svc := &Service{repo: stub, encKey: key}

	_, _, err := svc.Setup(context.Background(), "user1", "user1@example.com", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "current_code_required" {
		t.Errorf("got %q, want %q", err.Error(), "current_code_required")
	}
}

// TestSetup_MFAEnabled_WrongCode rejects an incorrect current TOTP code.
func TestSetup_MFAEnabled_WrongCode(t *testing.T) {
	key := testKey()
	totpSecret, _ := newTOTPKey(t)
	stub := &stubStore{enabled: true, secretEnc: encryptSecret(t, key, totpSecret)}
	svc := &Service{repo: stub, encKey: key}

	_, _, err := svc.Setup(context.Background(), "user1", "user1@example.com", "000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "invalid_code" {
		t.Errorf("got %q, want %q", err.Error(), "invalid_code")
	}
}

// TestSetup_MFAEnabled_CorrectCode accepts a valid current TOTP code and
// stores a new pending secret without touching the active secret.
func TestSetup_MFAEnabled_CorrectCode(t *testing.T) {
	key := testKey()
	totpSecret, currentCode := newTOTPKey(t)
	stub := &stubStore{enabled: true, secretEnc: encryptSecret(t, key, totpSecret)}
	svc := &Service{repo: stub, encKey: key}

	originalSecretEnc := stub.secretEnc

	url, secret, err := svc.Setup(context.Background(), "user1", "user1@example.com", currentCode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" || secret == "" {
		t.Error("expected non-empty setup result")
	}
	if stub.pendingEnc == "" {
		t.Error("UpsertPending was not called")
	}
	// Active secret must be unchanged — Setup must not touch secretEnc.
	if stub.secretEnc != originalSecretEnc {
		t.Error("active secret was mutated by Setup")
	}
}

// --- VerifyAndEnable tests ---

// TestVerifyAndEnable_ValidCode promotes a pending secret to active.
func TestVerifyAndEnable_ValidCode(t *testing.T) {
	key := testKey()
	totpSecret, code := newTOTPKey(t)
	stub := &stubStore{pendingEnc: encryptSecret(t, key, totpSecret)}
	svc := &Service{repo: stub, encKey: key}

	if err := svc.VerifyAndEnable(context.Background(), "user1", code); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !stub.enableCalled {
		t.Error("Enable was not called")
	}
}

// TestVerifyAndEnable_WrongCode rejects an incorrect code and does not call Enable.
func TestVerifyAndEnable_WrongCode(t *testing.T) {
	key := testKey()
	totpSecret, _ := newTOTPKey(t)
	stub := &stubStore{pendingEnc: encryptSecret(t, key, totpSecret)}
	svc := &Service{repo: stub, encKey: key}

	err := svc.VerifyAndEnable(context.Background(), "user1", "000000")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if stub.enableCalled {
		t.Error("Enable must not be called on wrong code")
	}
}

// TestVerifyAndEnable_NoPending returns an error when there is no pending setup.
func TestVerifyAndEnable_NoPending(t *testing.T) {
	stub := &stubStore{} // pendingEnc is empty → PendingSecretEnc returns ErrNotFound
	svc := &Service{repo: stub, encKey: testKey()}

	if err := svc.VerifyAndEnable(context.Background(), "user1", "123456"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

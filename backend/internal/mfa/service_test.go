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
	enabled       bool
	secretEnc     string // active encrypted secret
	pendingEnc    string // pending encrypted secret (written by UpsertPending)
	upsertErr     error
	enableErr     error
	deleteErr     error
	updateErr     error
	enableCalled  bool
	deleteCalled  bool
	recoveryCodes []string
	pendingCodes  []string
}

func (s *stubStore) IsEnabled(_ context.Context, _ string) bool { return s.enabled }

func (s *stubStore) UpsertPending(_ context.Context, _, enc string, pendingCodes []string) error {
	s.pendingEnc = enc
	s.pendingCodes = pendingCodes
	return s.upsertErr
}

func (s *stubStore) Enable(_ context.Context, _ string) error {
	s.enableCalled = true
	s.recoveryCodes = s.pendingCodes
	s.pendingCodes = nil
	return s.enableErr
}

func (s *stubStore) Delete(_ context.Context, _ string) error {
	s.deleteCalled = true
	return s.deleteErr
}

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

func (s *stubStore) RecoveryCodes(_ context.Context, _ string) ([]string, error) {
	return s.recoveryCodes, nil
}

func (s *stubStore) UpdateRecoveryCodes(_ context.Context, _ string, codes []string) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.recoveryCodes = codes
	return nil
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

	url, secret, recoveryCodes, err := svc.Setup(context.Background(), "user1", "user1@example.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" || secret == "" {
		t.Error("expected non-empty setup result")
	}
	if len(recoveryCodes) != 8 {
		t.Errorf("expected 8 recovery codes, got %d", len(recoveryCodes))
	}
	if stub.pendingEnc == "" {
		t.Error("UpsertPending was not called")
	}
	if len(stub.pendingCodes) != 8 {
		t.Error("UpsertPending did not store pending recovery codes")
	}
}

func TestSetup_MFAEnabled_EmptyCode(t *testing.T) {
	key := testKey()
	totpSecret, _ := newTOTPKey(t)
	stub := &stubStore{enabled: true, secretEnc: encryptSecret(t, key, totpSecret)}
	svc := &Service{repo: stub, encKey: key}

	_, _, _, err := svc.Setup(context.Background(), "user1", "user1@example.com", "")
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

	_, _, _, err := svc.Setup(context.Background(), "user1", "user1@example.com", "000000")
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

	url, secret, recoveryCodes, err := svc.Setup(context.Background(), "user1", "user1@example.com", currentCode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" || secret == "" {
		t.Error("expected non-empty setup result")
	}
	if len(recoveryCodes) != 8 {
		t.Error("expected 8 recovery codes")
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

// --- Recovery Codes validation tests ---

func TestRecoveryCodes_ValidationAndSingleUse(t *testing.T) {
	key := testKey()
	totpSecret, _ := newTOTPKey(t)
	stub := &stubStore{enabled: false}
	svc := &Service{repo: stub, encKey: key}

	// 1. Run Setup to generate pending secret and recovery codes
	_, _, rawCodes, err := svc.Setup(context.Background(), "user1", "user1@example.com", "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// 2. Enable MFA (VerifyAndEnable will promote the pending codes to active recovery codes)
	stub.pendingEnc = encryptSecret(t, key, totpSecret)
	code, _ := totp.GenerateCode(totpSecret, time.Now())
	if err := svc.VerifyAndEnable(context.Background(), "user1", code); err != nil {
		t.Fatalf("verify and enable failed: %v", err)
	}

	if len(stub.recoveryCodes) != 8 {
		t.Fatalf("expected 8 active recovery codes, got %d", len(stub.recoveryCodes))
	}

	// 3. Validate using a valid recovery code (should succeed)
	validCode := rawCodes[2]
	if !svc.Validate(context.Background(), "user1", validCode) {
		t.Error("expected valid recovery code to pass validation")
	}

	// 4. Recovery codes array should be updated to size 7 (single-use check)
	if len(stub.recoveryCodes) != 7 {
		t.Errorf("expected recovery codes size to decrease to 7, got %d", len(stub.recoveryCodes))
	}

	// 5. Trying to reuse the same recovery code must fail
	if svc.Validate(context.Background(), "user1", validCode) {
		t.Error("expected single-use recovery code reuse to fail")
	}

	// 6. Validating with an invalid code format/value must fail
	if svc.Validate(context.Background(), "user1", "invalid-code-here") {
		t.Error("expected invalid recovery code to fail validation")
	}
}

func TestDisable(t *testing.T) {
	t.Run("valid code deletes secret", func(t *testing.T) {
		key := testKey()
		secret, code := newTOTPKey(t)
		stub := &stubStore{secretEnc: encryptSecret(t, key, secret)}
		svc := &Service{repo: stub, encKey: key}

		if err := svc.Disable(context.Background(), "user1", code); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !stub.deleteCalled {
			t.Fatal("expected Delete to be called")
		}
	})

	t.Run("wrong code rejected", func(t *testing.T) {
		key := testKey()
		secret, _ := newTOTPKey(t)
		stub := &stubStore{secretEnc: encryptSecret(t, key, secret)}
		svc := &Service{repo: stub, encKey: key}

		err := svc.Disable(context.Background(), "user1", "000000")
		if err == nil || err.Error() != "invalid_code" {
			t.Fatalf("Disable error=%v want invalid_code", err)
		}
		if stub.deleteCalled {
			t.Fatal("Delete must not be called for invalid code")
		}
	})

	t.Run("delete error returned", func(t *testing.T) {
		key := testKey()
		secret, code := newTOTPKey(t)
		stub := &stubStore{secretEnc: encryptSecret(t, key, secret), deleteErr: ErrNotFound}
		svc := &Service{repo: stub, encKey: key}

		err := svc.Disable(context.Background(), "user1", code)
		if err != ErrNotFound {
			t.Fatalf("Disable error=%v want=%v", err, ErrNotFound)
		}
	})
}

func TestIsEnabledDelegatesToStore(t *testing.T) {
	svc := &Service{repo: &stubStore{enabled: true}, encKey: testKey()}
	if !svc.IsEnabled(context.Background(), "user1") {
		t.Fatal("expected IsEnabled to return store value")
	}
}

func TestValidateFailsWhenRecoveryCodesCannotBeUpdated(t *testing.T) {
	key := testKey()
	stub := &stubStore{updateErr: ErrNotFound}
	svc := &Service{repo: stub, encKey: key}

	_, _, rawCodes, err := svc.Setup(context.Background(), "user1", "user1@example.com", "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	stub.recoveryCodes = append([]string(nil), stub.pendingCodes...)

	if svc.Validate(context.Background(), "user1", rawCodes[0]) {
		t.Fatal("expected Validate to fail when recovery codes cannot be updated")
	}
}

func TestValidBase32(t *testing.T) {
	if !ValidBase32("JBSWY3DPEHPK3PXP") {
		t.Fatal("expected valid base32 secret to pass")
	}
	if ValidBase32("not-base32!") {
		t.Fatal("expected invalid base32 secret to fail")
	}
}

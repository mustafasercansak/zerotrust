package mfa

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/pquerna/otp/totp"

	appCrypto "github.com/zerotrust/backend/pkg/crypto"
)

// SetupResult is returned when the user initiates MFA setup.
type SetupResult struct {
	OTPAuthURL string // otpauth:// URI for authenticator apps
	Secret     string // raw base32 secret for manual entry
}

type Service struct {
	repo   *Repository
	encKey []byte // 32-byte AES-256 key
}

func NewService(repo *Repository, encKey []byte) *Service {
	return &Service{repo: repo, encKey: encKey}
}

// Setup generates a new TOTP secret, stores it encrypted, and returns the setup info.
// The record is not yet enabled — the user must call VerifyAndEnable with a valid code.
func (s *Service) Setup(ctx context.Context, userID, email string) (*SetupResult, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "ZeroTrust",
		AccountName: email,
	})
	if err != nil {
		return nil, err
	}

	enc, err := appCrypto.Encrypt(s.encKey, []byte(key.Secret()))
	if err != nil {
		return nil, err
	}

	if err := s.repo.Upsert(ctx, userID, hex.EncodeToString(enc)); err != nil {
		return nil, err
	}

	return &SetupResult{
		OTPAuthURL: key.URL(),
		Secret:     key.Secret(),
	}, nil
}

// VerifyAndEnable validates a TOTP code against the stored secret and, if valid,
// marks MFA as enabled for the user.
func (s *Service) VerifyAndEnable(ctx context.Context, userID, code string) error {
	secret, err := s.decryptSecret(ctx, userID)
	if err != nil {
		return err
	}
	if !totp.Validate(code, secret) {
		return errors.New("invalid_code")
	}
	return s.repo.Enable(ctx, userID)
}

// Disable removes TOTP from the user's account after verifying their current code.
func (s *Service) Disable(ctx context.Context, userID, code string) error {
	secret, err := s.decryptSecret(ctx, userID)
	if err != nil {
		return err
	}
	if !totp.Validate(code, secret) {
		return errors.New("invalid_code")
	}
	return s.repo.Delete(ctx, userID)
}

// IsEnabled satisfies auth.MFAChecker.
func (s *Service) IsEnabled(ctx context.Context, userID string) bool {
	return s.repo.IsEnabled(ctx, userID)
}

// Validate satisfies auth.MFAChecker.
func (s *Service) Validate(ctx context.Context, userID, code string) bool {
	secret, err := s.decryptSecret(ctx, userID)
	if err != nil {
		return false
	}
	return totp.Validate(code, secret)
}

func (s *Service) decryptSecret(ctx context.Context, userID string) (string, error) {
	encHex, err := s.repo.SecretEnc(ctx, userID)
	if err != nil {
		return "", err
	}
	enc, err := hex.DecodeString(encHex)
	if err != nil {
		return "", err
	}
	plain, err := appCrypto.Decrypt(s.encKey, enc)
	if err != nil {
		return "", err
	}
	// Normalise: pquerna/otp generates uppercase base32; ensure it stays uppercase.
	return strings.ToUpper(string(plain)), nil
}

// ValidBase32 reports whether s looks like a valid base32 TOTP secret.
func ValidBase32(s string) bool {
	_, err := base32.StdEncoding.DecodeString(strings.ToUpper(s))
	return err == nil
}

package mfa

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"

	appCrypto "github.com/zerotrust/backend/pkg/crypto"
)

// usedCodeTTL covers the ±1 TOTP window the library validates (3×30 s).
const usedCodeTTL = 90 * time.Second

// SetupResult is returned when the user initiates MFA setup.
type SetupResult struct {
	OTPAuthURL string // otpauth:// URI for authenticator apps
	Secret     string // raw base32 secret for manual entry
}

// store is the persistence interface consumed by Service.
// *Repository satisfies it; tests may supply a stub.
type store interface {
	IsEnabled(ctx context.Context, userID string) bool
	UpsertPending(ctx context.Context, userID, pendingEnc string) error
	Enable(ctx context.Context, userID string) error
	Delete(ctx context.Context, userID string) error
	SecretEnc(ctx context.Context, userID string) (string, error)
	PendingSecretEnc(ctx context.Context, userID string) (string, error)
}

type Service struct {
	repo   store
	encKey []byte // 32-byte AES-256 key
	rdb    *redis.Client
}

func NewService(repo *Repository, encKey []byte, rdb *redis.Client) *Service {
	return &Service{repo: repo, encKey: encKey, rdb: rdb}
}

// markUsed records a TOTP code as used for replay prevention.
// Returns false if the code was already used within usedCodeTTL.
// Returns true (fail-open) when Redis is unavailable.
func (s *Service) markUsed(ctx context.Context, userID, code string) bool {
	if s.rdb == nil {
		return true
	}
	key := fmt.Sprintf("mfa:used:%s:%s", userID, code)
	ok, err := s.rdb.SetNX(ctx, key, "1", usedCodeTTL).Result()
	return err != nil || ok
}

// Setup generates a new TOTP secret and stores it as a *pending* candidate.
// The active secret (and enabled_at) are untouched until VerifyAndEnable succeeds.
//
// If MFA is already enabled, currentCode must be the user's live TOTP code to
// prevent a stolen session from rotating MFA to an attacker-controlled device.
func (s *Service) Setup(ctx context.Context, userID, email, currentCode string) (*SetupResult, error) {
	if s.repo.IsEnabled(ctx, userID) {
		if currentCode == "" {
			return nil, errors.New("current_code_required")
		}
		secret, err := s.decryptSecret(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !totp.Validate(currentCode, secret) {
			return nil, errors.New("invalid_code")
		}
		if !s.markUsed(ctx, userID, currentCode) {
			return nil, errors.New("code_already_used")
		}
	}

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

	if err := s.repo.UpsertPending(ctx, userID, hex.EncodeToString(enc)); err != nil {
		return nil, err
	}

	return &SetupResult{
		OTPAuthURL: key.URL(),
		Secret:     key.Secret(),
	}, nil
}

// VerifyAndEnable validates a TOTP code against the *pending* secret and, if
// valid, atomically promotes it to the active secret. Returns an error if
// the code is wrong or there is no pending setup in progress.
func (s *Service) VerifyAndEnable(ctx context.Context, userID, code string) error {
	secret, err := s.decryptPendingSecret(ctx, userID)
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
	if !s.markUsed(ctx, userID, code) {
		return errors.New("code_already_used")
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
	if !totp.Validate(code, secret) {
		return false
	}
	return s.markUsed(ctx, userID, code)
}

func (s *Service) decryptSecret(ctx context.Context, userID string) (string, error) {
	encHex, err := s.repo.SecretEnc(ctx, userID)
	if err != nil {
		return "", err
	}
	return s.decryptHex(encHex)
}

func (s *Service) decryptPendingSecret(ctx context.Context, userID string) (string, error) {
	encHex, err := s.repo.PendingSecretEnc(ctx, userID)
	if err != nil {
		return "", err
	}
	return s.decryptHex(encHex)
}

func (s *Service) decryptHex(encHex string) (string, error) {
	enc, err := hex.DecodeString(encHex)
	if err != nil {
		return "", err
	}
	plain, err := appCrypto.Decrypt(s.encKey, enc)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(string(plain)), nil
}

// ValidBase32 reports whether s looks like a valid base32 TOTP secret.
func ValidBase32(s string) bool {
	_, err := base32.StdEncoding.DecodeString(strings.ToUpper(s))
	return err == nil
}

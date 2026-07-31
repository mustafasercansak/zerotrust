package mfa

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	appCrypto "github.com/zerotrust/backend/pkg/crypto"
)

// usedCodeTTL covers the ±1 TOTP window the library validates (3×30 s).
const usedCodeTTL = 90 * time.Second

// ErrReplayUnavailable is returned by high-risk operations (setup rotation,
// disable) when the Redis-backed TOTP replay check cannot be performed; those
// paths fail closed rather than disabling single-use enforcement (#102).
var ErrReplayUnavailable = errors.New("replay_protection_unavailable")

// SetupResult is returned when the user initiates MFA setup.
type SetupResult struct {
	OTPAuthURL    string   // otpauth:// URI for authenticator apps
	Secret        string   // raw base32 secret for manual entry
	RecoveryCodes []string // raw recovery codes to show to the user
}

// store is the persistence interface consumed by Service.
// *Repository satisfies it; tests may supply a stub.
type store interface {
	IsEnabled(ctx context.Context, userID string) bool
	UpsertPending(ctx context.Context, userID, pendingEnc string, pendingCodes []string) error
	Enable(ctx context.Context, userID string) error
	Delete(ctx context.Context, userID string) error
	SecretEnc(ctx context.Context, userID string) (string, error)
	PendingSecretEnc(ctx context.Context, userID string) (string, error)
	RecoveryCodes(ctx context.Context, userID string) ([]string, error)
	UpdateRecoveryCodes(ctx context.Context, userID string, codes []string) error
	// ConsumeRecoveryCode atomically finds and removes a recovery code,
	// returning true only when a match was consumed (#95). match is invoked
	// under a per-user lock and reports the index of the matching hash.
	ConsumeRecoveryCode(ctx context.Context, userID string, match func(hashes []string) (int, bool)) (bool, error)
}

type Service struct {
	repo     store
	encKey   []byte   // 32-byte AES-256 key
	prevKeys [][]byte // previous encryption keys, tried on decrypt failure (#103)
	rdb      *redis.Client
}

// NewService builds the MFA service. prevKeys are previously used encryption
// keys kept only so ciphertexts written before a key rotation can still be
// decrypted; new ciphertexts always use encKey (mirrors the JWT
// primary+secondary keystore).
func NewService(repo store, encKey []byte, rdb *redis.Client, prevKeys ...[]byte) *Service {
	return &Service{repo: repo, encKey: encKey, prevKeys: prevKeys, rdb: rdb}
}

// markUsed records a TOTP code as used for replay prevention.
// It returns used=true when the code was already seen within usedCodeTTL.
// A non-nil error means the replay check could not be performed (Redis down or
// not configured); callers decide whether to fail open (ordinary login, with a
// warning signal) or closed (step-up, disable, secret rotation) (#102).
func (s *Service) markUsed(ctx context.Context, userID, code string) (used bool, err error) {
	if s.rdb == nil {
		return false, ErrReplayUnavailable
	}
	key := fmt.Sprintf("mfa:used:%s:%s", userID, code)
	ok, err := s.rdb.SetNX(ctx, key, "1", usedCodeTTL).Result()
	if err != nil {
		return false, err
	}
	return !ok, nil
}

// Setup generates a new TOTP secret and stores it as a *pending* candidate.
// The active secret (and enabled_at) are untouched until VerifyAndEnable succeeds.
func (s *Service) Setup(ctx context.Context, userID, email, currentCode string) (string, string, []string, error) {
	if s.repo.IsEnabled(ctx, userID) {
		if currentCode == "" {
			return "", "", nil, errors.New("current_code_required")
		}
		secret, err := s.decryptSecret(ctx, userID)
		if err != nil {
			return "", "", nil, err
		}
		if !totp.Validate(currentCode, secret) {
			return "", "", nil, errors.New("invalid_code")
		}
		// Rotating the TOTP secret is high-risk: fail closed when the replay
		// check is unavailable (#102).
		used, err := s.markUsed(ctx, userID, currentCode)
		if err != nil {
			return "", "", nil, ErrReplayUnavailable
		}
		if used {
			return "", "", nil, errors.New("code_already_used")
		}
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "ZeroTrust",
		AccountName: email,
	})
	if err != nil {
		return "", "", nil, err
	}

	enc, err := appCrypto.Encrypt(s.encKey, []byte(key.Secret()))
	if err != nil {
		return "", "", nil, err
	}

	rawCodes, hashedCodes, err := generateRecoveryCodes()
	if err != nil {
		return "", "", nil, err
	}

	if err := s.repo.UpsertPending(ctx, userID, hex.EncodeToString(enc), hashedCodes); err != nil {
		return "", "", nil, err
	}

	return key.URL(), key.Secret(), rawCodes, nil
}

// VerifyAndEnable verifies a TOTP code and promoting the pending secret to active.
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
	// Disabling MFA is high-risk: fail closed when the replay check is
	// unavailable (#102).
	used, err := s.markUsed(ctx, userID, code)
	if err != nil {
		return ErrReplayUnavailable
	}
	if used {
		return errors.New("code_already_used")
	}
	return s.repo.Delete(ctx, userID)
}

// RegenerateRecoveryCodes invalidates current recovery codes and generates a set of 8 new ones.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	if !s.repo.IsEnabled(ctx, userID) {
		return nil, errors.New("mfa_disabled")
	}

	rawCodes, hashedCodes, err := generateRecoveryCodes()
	if err != nil {
		return nil, err
	}

	if err := s.repo.UpdateRecoveryCodes(ctx, userID, hashedCodes); err != nil {
		return nil, err
	}

	return rawCodes, nil
}

// IsEnabled satisfies auth.MFAChecker.
func (s *Service) IsEnabled(ctx context.Context, userID string) bool {
	return s.repo.IsEnabled(ctx, userID)
}

// Validate satisfies auth.MFAChecker. It is the ordinary-login path: when the
// Redis-backed replay check is unavailable it fails open (preserving login
// availability) but emits a warning signal each time (#102).
func (s *Service) Validate(ctx context.Context, userID, code string) bool {
	return s.validate(ctx, userID, code, false)
}

// ValidateStepUp is the high-risk variant of Validate used by step-up MFA:
// it fails closed when the replay check is unavailable (#102).
func (s *Service) ValidateStepUp(ctx context.Context, userID, code string) bool {
	return s.validate(ctx, userID, code, true)
}

func (s *Service) validate(ctx context.Context, userID, code string, failClosed bool) bool {
	code = strings.ToLower(strings.TrimSpace(code))

	// 1. Try TOTP validation first
	secret, err := s.decryptSecret(ctx, userID)
	if err == nil {
		if totp.Validate(code, secret) {
			used, err := s.markUsed(ctx, userID, code)
			if err != nil {
				slog.Warn("TOTP replay protection unavailable",
					"user_id", userID, "fail_closed", failClosed, "error", err)
				return !failClosed
			}
			return !used
		}
	}

	// 2. Check if it matches a recovery code. Consumption is atomic: the
	// repository holds a per-user lock while matching and removing the code,
	// so concurrent use of the same code succeeds exactly once (#95).
	consumed, err := s.repo.ConsumeRecoveryCode(ctx, userID, func(hashes []string) (int, bool) {
		for i, hashedCode := range hashes {
			if bcrypt.CompareHashAndPassword([]byte(hashedCode), []byte(code)) == nil {
				return i, true
			}
		}
		return -1, false
	})
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			slog.Error("failed to consume recovery code", "user_id", userID, "error", err)
		}
		return false
	}
	if consumed {
		slog.Info("MFA recovery code used successfully", "user_id", userID)
		return true
	}

	return false
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
		// Fall back to previous keys so ciphertexts written before a key
		// rotation remain readable (#103).
		for _, prev := range s.prevKeys {
			if plain, err = appCrypto.Decrypt(prev, enc); err == nil {
				break
			}
		}
		if err != nil {
			return "", err
		}
	}
	return strings.ToUpper(string(plain)), nil
}

// ValidBase32 reports whether s looks like a valid base32 TOTP secret.
func ValidBase32(s string) bool {
	_, err := base32.StdEncoding.DecodeString(strings.ToUpper(s))
	return err == nil
}

func generateRecoveryCodes() ([]string, []string, error) {
	rawCodes := make([]string, 8)
	hashedCodes := make([]string, 8)
	for i := 0; i < 8; i++ {
		b := make([]byte, 6)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		code := fmt.Sprintf("%04x-%04x-%04x", b[0:2], b[2:4], b[4:6])
		hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
		if err != nil {
			return nil, nil, err
		}
		rawCodes[i] = code
		hashedCodes[i] = string(hash)
	}
	return rawCodes, hashedCodes, nil
}

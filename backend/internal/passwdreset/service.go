package passwdreset

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/mailer"
)

var ErrPasswordReuseForbidden = errors.New("password_reuse_forbidden")

// resetCooldown is the minimum interval between reset emails sent to the same
// address. Without it, an attacker can mail-bomb a victim's inbox and keep
// invalidating any legitimate reset link (repo.Create cancels prior unused
// tokens). (ISSUE_LIST #93)
const resetCooldown = 5 * time.Minute

// store is the persistence interface consumed by Service.
// *Repository satisfies it; tests may supply a stub.
type store interface {
	Create(ctx context.Context, userID string) (string, error)
	ConsumeAndReset(ctx context.Context, rawToken, newPassword string) error
}

type Service struct {
	repo  store
	users userFinder
	mail  mailer.Mailer
	rdb   *redis.Client
}

type userFinder interface {
	FindByEmail(ctx context.Context, email string) (*user.User, error)
}

func NewService(repo store, users userFinder, mail mailer.Mailer) *Service {
	return &Service{repo: repo, users: users, mail: mail}
}

// SetRedis wires the per-email cooldown (ISSUE_LIST #93). Without it
// (rdb == nil) every request sends a fresh reset link, same as before.
func (s *Service) SetRedis(rdb *redis.Client) {
	s.rdb = rdb
}

// SendReset looks up the user and, if found, emails a reset link.
// Errors are intentionally swallowed — callers always return 200 to prevent enumeration.
func (s *Service) SendReset(ctx context.Context, email, baseURL string) error {
	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return nil // unknown email — stay silent
	}

	if s.rdb != nil {
		key := "pwdreset:cooldown:" + hashEmail(email)
		acquired, err := s.rdb.SetNX(ctx, key, "1", resetCooldown).Result()
		if err != nil {
			slog.Warn("password reset cooldown check unavailable, proceeding without throttle", "error", err)
		} else if !acquired {
			return nil // sent recently — stay silent, do not touch the existing token
		}
	}

	token, err := s.repo.Create(ctx, u.ID)
	if err != nil {
		return err
	}

	locale := u.Locale
	if locale == "" {
		locale = "en"
	}
	resetURL := fmt.Sprintf("%s/%s/auth/reset-password?token=%s", baseURL, locale, token)
	return s.mail.SendPasswordReset(ctx, email, resetURL)
}

// Reset validates the token, updates the password, and revokes all sessions in
// one atomic transaction. Token validation and bcrypt hashing both happen
// inside the repository, in that order, so an invalid token never triggers
// bcrypt work. (ISSUE_LIST #84) If any step fails the token is not consumed
// and the user can retry with the same link.
func (s *Service) Reset(ctx context.Context, rawToken, newPassword string) error {
	if err := s.repo.ConsumeAndReset(ctx, rawToken, newPassword); err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrExpired) || errors.Is(err, ErrUsed) {
			return errors.New("invalid_token")
		}
		return err
	}
	return nil
}

// hashEmail keys the Redis cooldown entry without storing the raw address.
func hashEmail(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h[:])
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

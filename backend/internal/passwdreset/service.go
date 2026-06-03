package passwdreset

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/mailer"
)

var ErrPasswordReuseForbidden = errors.New("password_reuse_forbidden")

// store is the persistence interface consumed by Service.
// *Repository satisfies it; tests may supply a stub.
type store interface {
	Create(ctx context.Context, userID string) (string, error)
	ConsumeAndReset(ctx context.Context, rawToken, newPassword, newPasswordHash string) error
}

type Service struct {
	repo  store
	users userFinder
	mail  mailer.Mailer
}

type userFinder interface {
	FindByEmail(ctx context.Context, email string) (*user.User, error)
}

func NewService(repo store, users userFinder, mail mailer.Mailer) *Service {
	return &Service{repo: repo, users: users, mail: mail}
}

// SendReset looks up the user and, if found, emails a reset link.
// Errors are intentionally swallowed — callers always return 200 to prevent enumeration.
func (s *Service) SendReset(ctx context.Context, email, baseURL string) error {
	u, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return nil // unknown email — stay silent
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
// one atomic transaction. If any step fails the token is not consumed and the
// user can retry with the same link.
func (s *Service) Reset(ctx context.Context, rawToken, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return err
	}

	if err := s.repo.ConsumeAndReset(ctx, rawToken, newPassword, string(hash)); err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrExpired) || errors.Is(err, ErrUsed) {
			return errors.New("invalid_token")
		}
		return err
	}
	return nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

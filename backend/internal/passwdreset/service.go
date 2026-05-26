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

type Service struct {
	repo  *Repository
	users *user.Service
	mail  mailer.Mailer
}

func NewService(repo *Repository, users *user.Service, mail mailer.Mailer) *Service {
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

	resetURL := fmt.Sprintf("%s/en/auth/reset-password?token=%s", baseURL, token)
	return s.mail.SendPasswordReset(ctx, email, resetURL)
}

// Reset validates the token and replaces the user's password with a bcrypt hash of newPassword.
func (s *Service) Reset(ctx context.Context, rawToken, newPassword string) error {
	userID, err := s.repo.Consume(ctx, rawToken)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrExpired) || errors.Is(err, ErrUsed) {
			return errors.New("invalid_token")
		}
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, userID, string(hash))
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

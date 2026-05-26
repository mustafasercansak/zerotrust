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

// SessionRevoker is implemented by session.Repository.
type SessionRevoker interface {
	RevokeAllForUser(ctx context.Context, userID string) error
}

type Service struct {
	repo     *Repository
	users    *user.Service
	mail     mailer.Mailer
	sessions SessionRevoker
}

func NewService(repo *Repository, users *user.Service, mail mailer.Mailer, sessions SessionRevoker) *Service {
	return &Service{repo: repo, users: users, mail: mail, sessions: sessions}
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

// Reset validates the token, replaces the user's password, and revokes all
// existing sessions so a compromised account cannot reuse stale refresh tokens.
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
	if err := s.users.UpdatePassword(ctx, userID, string(hash)); err != nil {
		return err
	}

	// Revoke all sessions so stale refresh tokens cannot be reused.
	_ = s.sessions.RevokeAllForUser(ctx, userID)
	return nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

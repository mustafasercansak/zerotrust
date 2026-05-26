package serviceaccount

import (
	"context"
	"time"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Create creates a new service account and returns it along with the plaintext secret.
// The secret is shown only once — store it immediately.
func (s *Service) Create(ctx context.Context, name, createdBy string, scopes []string, expiresAt *time.Time) (*ServiceAccount, string, error) {
	return s.repo.Create(ctx, name, createdBy, scopes, expiresAt)
}

func (s *Service) FindByClientID(ctx context.Context, clientID string) (*ServiceAccount, error) {
	return s.repo.FindByClientID(ctx, clientID)
}

func (s *Service) ListAll(ctx context.Context) ([]*ServiceAccount, error) {
	return s.repo.ListAll(ctx)
}

func (s *Service) Revoke(ctx context.Context, id string) error {
	return s.repo.Revoke(ctx, id)
}

func (s *Service) SetActive(ctx context.Context, id string, active bool) error {
	return s.repo.SetActive(ctx, id, active)
}

func (s *Service) CheckSecret(hash, secret string) bool {
	return s.repo.CheckSecret(hash, secret)
}

package serviceaccount

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zerotrust/backend/internal/auth"
)

var ErrUnknownScope = errors.New("unknown_scope")
var ErrForbiddenScope = errors.New("forbidden_scope")

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Create creates a new service account and returns it along with the plaintext secret.
// The secret is shown only once — store it immediately.
// Each requested scope must exist in the permissions table, and the caller must hold it.
func (s *Service) Create(ctx context.Context, name, createdBy string, caller *auth.Claims, scopes []string, expiresAt *time.Time) (*ServiceAccount, string, error) {
	if len(scopes) > 0 {
		known, err := s.repo.allPermissions(ctx)
		if err != nil {
			return nil, "", err
		}
		for _, scope := range scopes {
			if !known[scope] {
				return nil, "", fmt.Errorf("%w: %q", ErrUnknownScope, scope)
			}
			parts := strings.SplitN(scope, ":", 2)
			if !caller.HasPermission(parts[0], parts[1]) {
				return nil, "", fmt.Errorf("%w: %q", ErrForbiddenScope, scope)
			}
		}
	}
	return s.repo.Create(ctx, name, createdBy, scopes, expiresAt)
}

func (s *Service) FindByClientID(ctx context.Context, clientID string) (*ServiceAccount, error) {
	return s.repo.FindByClientID(ctx, clientID)
}

func (s *Service) List(ctx context.Context, p ListParams) (ListResult, error) {
	return s.repo.List(ctx, p)
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

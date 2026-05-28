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
	repo           *Repository
	scopeValidator interface {
		allPermissions(ctx context.Context) (map[string]bool, error)
	}
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo, scopeValidator: repo}
}

// Create creates a new service account and returns it along with the plaintext secret.
// The secret is shown only once — store it immediately.
// Each requested scope must exist in the permissions table, and the caller must hold it.
func (s *Service) Create(ctx context.Context, name, createdBy string, caller *auth.Claims, scopes []string, expiresAt *time.Time) (*ServiceAccount, string, error) {
	if err := s.validateScopes(ctx, caller, scopes); err != nil {
		return nil, "", err
	}
	return s.repo.Create(ctx, name, createdBy, scopes, expiresAt)
}

func (s *Service) Update(ctx context.Context, id, name string, caller *auth.Claims, scopes []string, expiresAt *time.Time, active bool) (*ServiceAccount, error) {
	if err := s.validateScopes(ctx, caller, scopes); err != nil {
		return nil, err
	}
	return s.repo.Update(ctx, id, name, scopes, expiresAt, active)
}

func (s *Service) validateScopes(ctx context.Context, caller *auth.Claims, scopes []string) error {
	if len(scopes) > 0 {
		known, err := s.scopeValidator.allPermissions(ctx)
		if err != nil {
			return err
		}
		for _, scope := range scopes {
			if !known[scope] {
				return fmt.Errorf("%w: %q", ErrUnknownScope, scope)
			}
			parts := strings.SplitN(scope, ":", 2)
			if caller == nil || len(parts) != 2 {
				return fmt.Errorf("%w: %q", ErrForbiddenScope, scope)
			}
			if !caller.HasPermission(parts[0], parts[1]) {
				return fmt.Errorf("%w: %q", ErrForbiddenScope, scope)
			}
		}
	}
	return nil
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

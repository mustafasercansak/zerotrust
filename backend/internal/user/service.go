package user

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Register(ctx context.Context, email, password, locale string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, email, string(hash), locale)
}

func (s *Service) RegisterWithRoles(ctx context.Context, email, password, locale string, roles []string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return s.repo.CreateWithRoles(ctx, email, string(hash), locale, roles)
}

func (s *Service) FindByEmail(ctx context.Context, email string) (*User, error) {
	return s.repo.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
}

func (s *Service) FindByID(ctx context.Context, id string) (*User, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *Service) CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (s *Service) ListAll(ctx context.Context) ([]*User, error) {
	return s.repo.ListAll(ctx)
}

func (s *Service) SetRoles(ctx context.Context, userID string, roles []string) error {
	return s.repo.SetRoles(ctx, userID, roles)
}

func (s *Service) GetPermissions(ctx context.Context, userID string) ([]string, error) {
	return s.repo.GetPermissions(ctx, userID)
}

func (s *Service) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	return s.repo.UpdatePassword(ctx, userID, passwordHash)
}

// SeedAdmin creates the initial admin user with the admin role if the email doesn't exist.
// hash must be a pre-computed bcrypt hash.
func (s *Service) SeedAdmin(ctx context.Context, email, hash string) error {
	u, err := s.repo.FindByEmail(ctx, email)
	if err == nil {
		// Already exists — ensure admin role is assigned.
		return s.repo.AssignRoleByName(ctx, u.ID, "admin")
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	u, err = s.repo.Create(ctx, email, hash, "tr")
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			return nil // concurrent seed
		}
		return err
	}
	return s.repo.AssignRoleByName(ctx, u.ID, "admin")
}

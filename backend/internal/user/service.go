package user

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

type store interface {
	Create(ctx context.Context, email, passwordHash, locale string) (*User, error)
	CreateWithRoles(ctx context.Context, email, passwordHash, locale string, roles []string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByID(ctx context.Context, id string) (*User, error)
	List(ctx context.Context, p ListParams) (ListResult, error)
	SetRoles(ctx context.Context, userID string, roles []string) error
	SetActive(ctx context.Context, userID string, active bool) error
	BulkSetActive(ctx context.Context, userIDs []string, active bool) error
	UpdateProfile(ctx context.Context, userID, firstName, lastName string) (*User, error)
	GetPermissions(ctx context.Context, userID string) ([]string, error)
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	AssignRoleByName(ctx context.Context, userID, roleName string) error
	UpdateAvatar(ctx context.Context, userID, key string, size int) (*User, error)
}

type Service struct {
	repo store
}

func NewService(repo store) *Service {
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

func (s *Service) List(ctx context.Context, p ListParams) (ListResult, error) {
	return s.repo.List(ctx, p)
}

func (s *Service) SetRoles(ctx context.Context, userID string, roles []string) error {
	return s.repo.SetRoles(ctx, userID, roles)
}

func (s *Service) SetActive(ctx context.Context, userID string, active bool) error {
	return s.repo.SetActive(ctx, userID, active)
}

func (s *Service) BulkSetActive(ctx context.Context, userIDs []string, active bool) error {
	return s.repo.BulkSetActive(ctx, userIDs, active)
}

func (s *Service) UpdateProfile(ctx context.Context, userID, firstName, lastName string) (*User, error) {
	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)
	if len([]rune(firstName)) > 80 || len([]rune(lastName)) > 80 {
		return nil, ErrInvalidProfile
	}
	return s.repo.UpdateProfile(ctx, userID, firstName, lastName)
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

func (s *Service) UpdateAvatar(ctx context.Context, userID, key string, size int) (*User, error) {
	return s.repo.UpdateAvatar(ctx, userID, key, size)
}

package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zerotrust/backend/internal/user"
)

type clientCredentialsUserReader struct{}

func (r *clientCredentialsUserReader) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	return nil, user.ErrNotFound
}

func (r *clientCredentialsUserReader) FindByID(ctx context.Context, id string) (*user.User, error) {
	return nil, user.ErrNotFound
}

func (r *clientCredentialsUserReader) CheckPassword(hash, password string) bool {
	return false
}

func (r *clientCredentialsUserReader) GetPermissions(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

type clientCredentialsStore struct {
	record *ServiceAccountRecord
	err    error
}

func (s *clientCredentialsStore) FindByClientID(ctx context.Context, clientID string) (*ServiceAccountRecord, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.record, nil
}

func (s *clientCredentialsStore) CheckSecret(hash, secret string) bool {
	if hash == "valid-hash" && secret == "valid-secret" {
		return true
	}
	if hash == "old-hash" && secret == "old-secret" {
		return true
	}
	return false
}

func newClientCredentialsService(t *testing.T, store ServiceAccountStore) *Service {
	t.Helper()
	ks, err := LoadOrGenerateKeyStore("", "")
	if err != nil {
		t.Fatalf("key store: %v", err)
	}
	return NewService(&clientCredentialsUserReader{}, nil, store, nil, ks, nil, nil)
}

func TestClientCredentialsIssuesTokenWithStoredScopes(t *testing.T) {
	store := &clientCredentialsStore{record: &ServiceAccountRecord{
		Name:             "Deploy Bot",
		ClientSecretHash: "valid-hash",
		Scopes:           []string{"users:read", "audit:read"},
		IsActive:         true,
	}}
	svc := newClientCredentialsService(t, store)

	resp, err := svc.ClientCredentials(context.Background(), "svc_123", "valid-secret", "")
	if err != nil {
		t.Fatalf("ClientCredentials returned error: %v", err)
	}
	claims, err := ValidateAccessToken(svc.ks, resp.AccessToken)
	if err != nil {
		t.Fatalf("validate service token: %v", err)
	}
	if claims.SubType != SubTypeService {
		t.Fatalf("SubType=%q want %q", claims.SubType, SubTypeService)
	}
	if claims.ClientID != "svc_123" {
		t.Fatalf("ClientID=%q want svc_123", claims.ClientID)
	}
	if claims.Subject != "Deploy Bot" {
		t.Fatalf("Subject=%q want Deploy Bot", claims.Subject)
	}
	if !claims.HasPermission("users", "read") || !claims.HasPermission("audit", "read") {
		t.Fatalf("service token scopes=%v missing expected permissions", claims.Scopes)
	}
	if claims.HasPermission("users", "delete") {
		t.Fatalf("service token unexpectedly grants users:delete: %v", claims.Scopes)
	}
}

func TestClientCredentialsRejectsExpiredAccount(t *testing.T) {
	expired := time.Now().Add(-time.Minute)
	store := &clientCredentialsStore{record: &ServiceAccountRecord{
		Name:             "Expired Bot",
		ClientSecretHash: "valid-hash",
		Scopes:           []string{"users:read"},
		IsActive:         true,
		ExpiresAt:        &expired,
	}}
	svc := newClientCredentialsService(t, store)

	_, err := svc.ClientCredentials(context.Background(), "svc_expired", "valid-secret", "")
	if !errors.Is(err, ErrInactiveUser) {
		t.Fatalf("ClientCredentials error=%v want ErrInactiveUser", err)
	}
}

func TestClientCredentialsRejectsInactiveAccount(t *testing.T) {
	store := &clientCredentialsStore{record: &ServiceAccountRecord{
		Name:             "Inactive Bot",
		ClientSecretHash: "valid-hash",
		Scopes:           []string{"users:read"},
		IsActive:         false,
	}}
	svc := newClientCredentialsService(t, store)

	_, err := svc.ClientCredentials(context.Background(), "svc_inactive", "valid-secret", "")
	if !errors.Is(err, ErrInactiveUser) {
		t.Fatalf("ClientCredentials error=%v want ErrInactiveUser", err)
	}
}

func TestClientCredentialsReflectsStatusChanges(t *testing.T) {
	record := &ServiceAccountRecord{
		Name:             "Toggle Bot",
		ClientSecretHash: "valid-hash",
		Scopes:           []string{"users:read"},
		IsActive:         true,
	}
	store := &clientCredentialsStore{record: record}
	svc := newClientCredentialsService(t, store)

	if _, err := svc.ClientCredentials(context.Background(), "svc_toggle", "valid-secret", ""); err != nil {
		t.Fatalf("active service account should issue token, got error: %v", err)
	}

	record.IsActive = false
	_, err := svc.ClientCredentials(context.Background(), "svc_toggle", "valid-secret", "")
	if !errors.Is(err, ErrInactiveUser) {
		t.Fatalf("inactive service account error=%v want ErrInactiveUser", err)
	}
}

func TestClientCredentialsRejectsInvalidSecret(t *testing.T) {
	store := &clientCredentialsStore{record: &ServiceAccountRecord{
		Name:             "Deploy Bot",
		ClientSecretHash: "valid-hash",
		Scopes:           []string{"users:read"},
		IsActive:         true,
	}}
	svc := newClientCredentialsService(t, store)

	_, err := svc.ClientCredentials(context.Background(), "svc_123", "wrong-secret", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("ClientCredentials error=%v want ErrInvalidCredentials", err)
	}
}

func TestClientCredentialsAcceptsOldSecretWithinGracePeriod(t *testing.T) {
	future := time.Now().Add(30 * time.Minute)
	oldHash := "old-hash"
	store := &clientCredentialsStore{record: &ServiceAccountRecord{
		Name:                "Rotated Bot",
		ClientSecretHash:    "valid-hash",
		Scopes:              []string{"users:read"},
		IsActive:            true,
		OldClientSecretHash: &oldHash,
		OldSecretExpiresAt:  &future,
	}}
	svc := newClientCredentialsService(t, store)

	// New secret works
	if _, err := svc.ClientCredentials(context.Background(), "svc_123", "valid-secret", ""); err != nil {
		t.Fatalf("new secret failed: %v", err)
	}

	// Old secret works within grace period
	if _, err := svc.ClientCredentials(context.Background(), "svc_123", "old-secret", ""); err != nil {
		t.Fatalf("old secret within grace period failed: %v", err)
	}
}

func TestClientCredentialsRejectsOldSecretAfterGracePeriod(t *testing.T) {
	past := time.Now().Add(-5 * time.Minute)
	oldHash := "old-hash"
	store := &clientCredentialsStore{record: &ServiceAccountRecord{
		Name:                "Rotated Bot",
		ClientSecretHash:    "valid-hash",
		Scopes:              []string{"users:read"},
		IsActive:            true,
		OldClientSecretHash: &oldHash,
		OldSecretExpiresAt:  &past,
	}}
	svc := newClientCredentialsService(t, store)

	// Old secret fails after grace period
	_, err := svc.ClientCredentials(context.Background(), "svc_123", "old-secret", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for expired old secret, got: %v", err)
	}
}

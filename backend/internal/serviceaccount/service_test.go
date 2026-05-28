package serviceaccount

import (
	"context"
	"errors"
	"testing"

	"github.com/zerotrust/backend/internal/auth"
)

type scopePolicyRepo struct {
	permissions map[string]bool
	createCalls int
	updateCalls int
}

func (r *scopePolicyRepo) allPermissions(ctx context.Context) (map[string]bool, error) {
	return r.permissions, nil
}

func TestValidateScopesAllowsOnlyKnownScopesHeldByCaller(t *testing.T) {
	repo := &scopePolicyRepo{permissions: map[string]bool{
		"users:read": true,
		"audit:read": true,
	}}
	svc := &Service{scopeValidator: repo}
	caller := &auth.Claims{Permissions: []string{"users:read", "audit:read"}}

	if err := svc.validateScopes(context.Background(), caller, []string{"users:read", "audit:read"}); err != nil {
		t.Fatalf("validateScopes returned error: %v", err)
	}
}

func TestValidateScopesRejectsUnknownScope(t *testing.T) {
	repo := &scopePolicyRepo{permissions: map[string]bool{"users:read": true}}
	svc := &Service{scopeValidator: repo}
	caller := &auth.Claims{Permissions: []string{"users:read", "service_accounts:delete"}}

	err := svc.validateScopes(context.Background(), caller, []string{"service_accounts:delete"})
	if !errors.Is(err, ErrUnknownScope) {
		t.Fatalf("validateScopes error=%v want ErrUnknownScope", err)
	}
}

func TestValidateScopesRejectsScopeCallerDoesNotHold(t *testing.T) {
	repo := &scopePolicyRepo{permissions: map[string]bool{
		"users:read":   true,
		"users:delete": true,
	}}
	svc := &Service{scopeValidator: repo}
	caller := &auth.Claims{Permissions: []string{"users:read"}}

	err := svc.validateScopes(context.Background(), caller, []string{"users:delete"})
	if !errors.Is(err, ErrForbiddenScope) {
		t.Fatalf("validateScopes error=%v want ErrForbiddenScope", err)
	}
}

func TestValidateScopesRejectsMissingCaller(t *testing.T) {
	repo := &scopePolicyRepo{permissions: map[string]bool{"users:read": true}}
	svc := &Service{scopeValidator: repo}

	err := svc.validateScopes(context.Background(), nil, []string{"users:read"})
	if !errors.Is(err, ErrForbiddenScope) {
		t.Fatalf("validateScopes error=%v want ErrForbiddenScope", err)
	}
}

package serviceaccount

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zerotrust/backend/internal/auth"
)

type scopePolicyRepo struct {
	permissions map[string]bool
	err         error
	createCalls int
	updateCalls int
}

func (r *scopePolicyRepo) allPermissions(ctx context.Context) (map[string]bool, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.permissions, nil
}

type fakeServiceAccountRepo struct {
	createSA         *ServiceAccount
	createSecret     string
	createErr        error
	createName       string
	createCreatedBy  string
	createScopes     []string
	createExpiresAt  *time.Time
	updateSA         *ServiceAccount
	updateErr        error
	updateID         string
	updateName       string
	updateScopes     []string
	updateExpiresAt  *time.Time
	updateActive     bool
	findResp         *ServiceAccount
	findErr          error
	findClientID     string
	listResp         ListResult
	listErr          error
	listParams       ListParams
	revokeErr        error
	revokeID         string
	setActiveErr     error
	setActiveID      string
	setActiveValue   bool
	checkSecretResp  bool
	checkSecretHash  string
	checkSecretValue string
	rotateResp       *ServiceAccount
	rotateSecret     string
	rotateErr        error
	rotateID         string
}

func (r *fakeServiceAccountRepo) Create(ctx context.Context, name, createdBy string, scopes []string, expiresAt *time.Time) (*ServiceAccount, string, error) {
	r.createName = name
	r.createCreatedBy = createdBy
	r.createScopes = append([]string(nil), scopes...)
	r.createExpiresAt = expiresAt
	return r.createSA, r.createSecret, r.createErr
}

func (r *fakeServiceAccountRepo) Update(ctx context.Context, id, name string, scopes []string, expiresAt *time.Time, active bool) (*ServiceAccount, error) {
	r.updateID = id
	r.updateName = name
	r.updateScopes = append([]string(nil), scopes...)
	r.updateExpiresAt = expiresAt
	r.updateActive = active
	return r.updateSA, r.updateErr
}

func (r *fakeServiceAccountRepo) FindByClientID(ctx context.Context, clientID string) (*ServiceAccount, error) {
	r.findClientID = clientID
	return r.findResp, r.findErr
}

func (r *fakeServiceAccountRepo) List(ctx context.Context, p ListParams) (ListResult, error) {
	r.listParams = p
	return r.listResp, r.listErr
}

func (r *fakeServiceAccountRepo) Revoke(ctx context.Context, id string) error {
	r.revokeID = id
	return r.revokeErr
}

func (r *fakeServiceAccountRepo) SetActive(ctx context.Context, id string, active bool) error {
	r.setActiveID = id
	r.setActiveValue = active
	return r.setActiveErr
}

func (r *fakeServiceAccountRepo) CheckSecret(hash, secret string) bool {
	r.checkSecretHash = hash
	r.checkSecretValue = secret
	return r.checkSecretResp
}

func (r *fakeServiceAccountRepo) RotateSecret(ctx context.Context, id string) (*ServiceAccount, string, error) {
	r.rotateID = id
	return r.rotateResp, r.rotateSecret, r.rotateErr
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

func TestValidateScopesAllowsEmptyScopeList(t *testing.T) {
	svc := &Service{scopeValidator: &scopePolicyRepo{err: errors.New("should not be called")}}

	if err := svc.validateScopes(context.Background(), nil, nil); err != nil {
		t.Fatalf("validateScopes returned error: %v", err)
	}
}

func TestValidateScopesPropagatesPermissionLookupError(t *testing.T) {
	wantErr := errors.New("permissions unavailable")
	svc := &Service{scopeValidator: &scopePolicyRepo{err: wantErr}}
	caller := &auth.Claims{Permissions: []string{"users:read"}}

	err := svc.validateScopes(context.Background(), caller, []string{"users:read"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("validateScopes error=%v want=%v", err, wantErr)
	}
}

func TestValidateScopesRejectsMalformedScope(t *testing.T) {
	repo := &scopePolicyRepo{permissions: map[string]bool{"malformed": true}}
	svc := &Service{scopeValidator: repo}
	caller := &auth.Claims{Permissions: []string{"malformed"}}

	err := svc.validateScopes(context.Background(), caller, []string{"malformed"})
	if !errors.Is(err, ErrForbiddenScope) {
		t.Fatalf("validateScopes error=%v want ErrForbiddenScope", err)
	}
}

func TestNewServiceUsesRepositoryForValidation(t *testing.T) {
	repo := &Repository{}
	svc := NewService(repo)

	if svc.repo != repo {
		t.Fatal("expected NewService to store repository dependency")
	}
	if svc.scopeValidator != repo {
		t.Fatal("expected NewService to reuse repository as scope validator")
	}
}

func TestServiceCreateValidatesScopesAndCallsRepository(t *testing.T) {
	expiresAt := time.Now().UTC().Truncate(time.Second)
	repo := &fakeServiceAccountRepo{
		createSA:     &ServiceAccount{ID: "sa1"},
		createSecret: "plain-secret",
	}
	svc := &Service{
		repo:           repo,
		scopeValidator: &scopePolicyRepo{permissions: map[string]bool{"users:read": true}},
	}
	caller := &auth.Claims{Permissions: []string{"users:read"}}

	sa, secret, err := svc.Create(context.Background(), "svc", "admin", caller, []string{"users:read"}, &expiresAt)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if sa != repo.createSA || secret != "plain-secret" {
		t.Fatalf("Create returned unexpected values sa=%v secret=%q", sa, secret)
	}
	if repo.createName != "svc" || repo.createCreatedBy != "admin" {
		t.Fatalf("unexpected create args name=%q createdBy=%q", repo.createName, repo.createCreatedBy)
	}
	if len(repo.createScopes) != 1 || repo.createScopes[0] != "users:read" {
		t.Fatalf("unexpected create scopes: %#v", repo.createScopes)
	}
	if repo.createExpiresAt != &expiresAt {
		t.Fatal("expected create expiresAt pointer to be passed through")
	}
}

func TestServiceCreateReturnsValidationErrorWithoutCallingRepository(t *testing.T) {
	repo := &fakeServiceAccountRepo{}
	svc := &Service{
		repo:           repo,
		scopeValidator: &scopePolicyRepo{permissions: map[string]bool{"users:read": true}},
	}
	caller := &auth.Claims{Permissions: []string{"audit:read"}}

	_, _, err := svc.Create(context.Background(), "svc", "admin", caller, []string{"users:read"}, nil)
	if !errors.Is(err, ErrForbiddenScope) {
		t.Fatalf("Create error=%v want ErrForbiddenScope", err)
	}
	if repo.createName != "" {
		t.Fatal("repository should not be called when validation fails")
	}
}

func TestServiceUpdateValidatesScopesAndCallsRepository(t *testing.T) {
	expiresAt := time.Now().UTC().Truncate(time.Second)
	repo := &fakeServiceAccountRepo{updateSA: &ServiceAccount{ID: "sa1", Name: "updated"}}
	svc := &Service{
		repo:           repo,
		scopeValidator: &scopePolicyRepo{permissions: map[string]bool{"users:read": true}},
	}
	caller := &auth.Claims{Permissions: []string{"users:read"}}

	sa, err := svc.Update(context.Background(), "sa1", "updated", caller, []string{"users:read"}, &expiresAt, true)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if sa != repo.updateSA {
		t.Fatalf("Update returned unexpected account: %+v", sa)
	}
	if repo.updateID != "sa1" || repo.updateName != "updated" || !repo.updateActive {
		t.Fatalf("unexpected update args id=%q name=%q active=%v", repo.updateID, repo.updateName, repo.updateActive)
	}
	if repo.updateExpiresAt != &expiresAt {
		t.Fatal("expected update expiresAt pointer to be passed through")
	}
}

func TestServicePassthroughMethods(t *testing.T) {
	wantSA := &ServiceAccount{ID: "sa1"}
	repo := &fakeServiceAccountRepo{
		findResp:        wantSA,
		listResp:        ListResult{Accounts: []*ServiceAccount{wantSA}, Total: 1},
		checkSecretResp: true,
		rotateResp:      wantSA,
		rotateSecret:    "new-secret",
	}
	svc := &Service{repo: repo}

	gotSA, err := svc.FindByClientID(context.Background(), "client-1")
	if err != nil || gotSA != wantSA {
		t.Fatalf("FindByClientID got (%v, %v) want (%v, nil)", gotSA, err, wantSA)
	}

	list, err := svc.List(context.Background(), ListParams{Limit: 5, Offset: 10})
	if err != nil || list.Total != 1 || len(list.Accounts) != 1 {
		t.Fatalf("List got (%+v, %v)", list, err)
	}

	if err := svc.Revoke(context.Background(), "sa1"); err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}

	if err := svc.SetActive(context.Background(), "sa1", true); err != nil {
		t.Fatalf("SetActive returned error: %v", err)
	}

	if ok := svc.CheckSecret("hash", "secret"); !ok {
		t.Fatal("CheckSecret returned false want true")
	}

	rotated, secret, err := svc.RotateSecret(context.Background(), "sa1")
	if err != nil || rotated != wantSA || secret != "new-secret" {
		t.Fatalf("RotateSecret got (%v, %q, %v)", rotated, secret, err)
	}

	if repo.findClientID != "client-1" {
		t.Fatalf("unexpected FindByClientID arg: %q", repo.findClientID)
	}
	if repo.listParams.Limit != 5 || repo.listParams.Offset != 10 {
		t.Fatalf("unexpected list params: %+v", repo.listParams)
	}
	if repo.revokeID != "sa1" || repo.setActiveID != "sa1" || !repo.setActiveValue {
		t.Fatalf("unexpected revoke/setactive args revoke=%q setActive=%q active=%v", repo.revokeID, repo.setActiveID, repo.setActiveValue)
	}
	if repo.checkSecretHash != "hash" || repo.checkSecretValue != "secret" {
		t.Fatalf("unexpected CheckSecret args hash=%q secret=%q", repo.checkSecretHash, repo.checkSecretValue)
	}
	if repo.rotateID != "sa1" {
		t.Fatalf("unexpected RotateSecret arg: %q", repo.rotateID)
	}
}

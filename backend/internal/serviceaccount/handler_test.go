package serviceaccount

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/pkg/middleware"
)

func newEventTestKeyStore(t *testing.T) *auth.KeyStore {
	t.Helper()
	ks, err := auth.LoadOrGenerateKeyStore("", "")
	if err != nil {
		t.Fatalf("key store: %v", err)
	}
	return ks
}

func newServiceAccountReadToken(t *testing.T, ks *auth.KeyStore) string {
	t.Helper()
	pair, err := auth.GenerateTokenPair(
		ks,
		"user-1",
		"admin@example.com",
		"en",
		[]string{"admin"},
		[]string{"service_accounts:read"},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return pair.AccessToken
}

func TestEventsRejectsQueryToken(t *testing.T) {
	ks := newEventTestKeyStore(t)
	token := newServiceAccountReadToken(t, ks)
	h := NewHandler(nil, NewEventHub(), ks, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts/events?token="+token, nil)
	rr := httptest.NewRecorder()

	h.Events(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
}

func TestEventsAcceptsAuthorizationBearerToken(t *testing.T) {
	ks := newEventTestKeyStore(t)
	token := newServiceAccountReadToken(t, ks)
	h := NewHandler(nil, NewEventHub(), ks, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts/events", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	h.Events(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type=%q want text/event-stream", got)
	}
	if body := rr.Body.String(); body != "data: connected\n\n" {
		t.Fatalf("body=%q want connected event", body)
	}
}

func TestEventsAcceptsAccessTokenCookie(t *testing.T) {
	ks := newEventTestKeyStore(t)
	token := newServiceAccountReadToken(t, ks)
	h := NewHandler(nil, NewEventHub(), ks, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts/events", nil).WithContext(ctx)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	rr := httptest.NewRecorder()

	h.Events(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

type fakeServiceAccountService struct {
	createErr      error
	updateErr      error
	revokeErr      error
	setActiveErr   error
	createdScopes  []string
	updatedScopes  []string
	setActiveValue bool
}

func (s *fakeServiceAccountService) Create(ctx context.Context, name, createdBy string, caller *auth.Claims, scopes []string, expiresAt *time.Time) (*ServiceAccount, string, error) {
	s.createdScopes = scopes
	if s.createErr != nil {
		return nil, "", s.createErr
	}
	return &ServiceAccount{ID: "sa1", Name: name, ClientID: "svc_1", IsActive: true, Scopes: scopes, CreatedAt: time.Now()}, "secret", nil
}

func (s *fakeServiceAccountService) Update(ctx context.Context, id, name string, caller *auth.Claims, scopes []string, expiresAt *time.Time, active bool) (*ServiceAccount, error) {
	s.updatedScopes = scopes
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return &ServiceAccount{ID: id, Name: name, ClientID: "svc_1", IsActive: active, Scopes: scopes, CreatedAt: time.Now()}, nil
}

func (s *fakeServiceAccountService) List(ctx context.Context, p ListParams) (ListResult, error) {
	return ListResult{}, nil
}

func (s *fakeServiceAccountService) Revoke(ctx context.Context, id string) error {
	return s.revokeErr
}

func (s *fakeServiceAccountService) SetActive(ctx context.Context, id string, active bool) error {
	s.setActiveValue = active
	return s.setActiveErr
}

func requestWithClaims(req *http.Request) *http.Request {
	claims := &auth.Claims{Permissions: []string{"users:read", "service_accounts:create", "service_accounts:update"}}
	ctx := context.WithValue(req.Context(), middleware.ClaimsKey, claims)
	return req.WithContext(ctx)
}

func requestWithURLParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestCreateMapsScopePolicyErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
		body string
	}{
		{name: "unknown scope", err: ErrUnknownScope, want: http.StatusUnprocessableEntity, body: "unknown_scope"},
		{name: "forbidden scope", err: ErrForbiddenScope, want: http.StatusForbidden, body: "forbidden_scope"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{svc: &fakeServiceAccountService{createErr: tc.err}, hub: NewEventHub()}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-accounts", bytes.NewReader([]byte(`{"name":"bot","scopes":["users:delete"]}`)))
			rr := httptest.NewRecorder()

			h.Create(rr, requestWithClaims(req))

			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.want, rr.Body.String())
			}
			if !bytes.Contains(rr.Body.Bytes(), []byte(tc.body)) {
				t.Fatalf("body=%s want %q", rr.Body.String(), tc.body)
			}
		})
	}
}

func TestSetStatusAndRevokeReturnNotFound(t *testing.T) {
	h := &Handler{svc: &fakeServiceAccountService{setActiveErr: ErrNotFound, revokeErr: ErrNotFound}, hub: NewEventHub()}

	statusReq := requestWithURLParam(
		httptest.NewRequest(http.MethodPatch, "/api/v1/admin/service-accounts/sa1/status", bytes.NewReader([]byte(`{"is_active":false}`))),
		"id",
		"sa1",
	)
	statusRR := httptest.NewRecorder()
	h.SetStatus(statusRR, statusReq)
	if statusRR.Code != http.StatusNotFound {
		t.Fatalf("SetStatus status=%d want=%d body=%s", statusRR.Code, http.StatusNotFound, statusRR.Body.String())
	}

	revokeReq := requestWithURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/service-accounts/sa1", nil), "id", "sa1")
	revokeRR := httptest.NewRecorder()
	h.Revoke(revokeRR, revokeReq)
	if revokeRR.Code != http.StatusNotFound {
		t.Fatalf("Revoke status=%d want=%d body=%s", revokeRR.Code, http.StatusNotFound, revokeRR.Body.String())
	}
}

func TestSetStatusSuccessUsesRequestedActiveValue(t *testing.T) {
	svc := &fakeServiceAccountService{}
	h := &Handler{svc: svc, hub: NewEventHub()}
	req := requestWithURLParam(
		httptest.NewRequest(http.MethodPatch, "/api/v1/admin/service-accounts/sa1/status", bytes.NewReader([]byte(`{"is_active":true}`))),
		"id",
		"sa1",
	)
	rr := httptest.NewRecorder()

	h.SetStatus(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
	if !svc.setActiveValue {
		t.Fatal("SetActive did not receive requested active=true value")
	}
}

func TestUpdateMapsNotFoundAndScopePolicyErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
		body string
	}{
		{name: "not found", err: ErrNotFound, want: http.StatusNotFound, body: "not_found"},
		{name: "unknown scope", err: ErrUnknownScope, want: http.StatusUnprocessableEntity, body: "unknown_scope"},
		{name: "forbidden scope", err: ErrForbiddenScope, want: http.StatusForbidden, body: "forbidden_scope"},
		{name: "internal", err: errors.New("db down"), want: http.StatusInternalServerError, body: "internal_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{svc: &fakeServiceAccountService{updateErr: tc.err}, hub: NewEventHub()}
			req := requestWithURLParam(
				httptest.NewRequest(http.MethodPatch, "/api/v1/admin/service-accounts/sa1", bytes.NewReader([]byte(`{"name":"bot","scopes":["users:delete"],"is_active":true}`))),
				"id",
				"sa1",
			)
			rr := httptest.NewRecorder()

			h.Update(rr, requestWithClaims(req))

			if rr.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.want, rr.Body.String())
			}
			if !bytes.Contains(rr.Body.Bytes(), []byte(tc.body)) {
				t.Fatalf("body=%s want %q", rr.Body.String(), tc.body)
			}
		})
	}
}

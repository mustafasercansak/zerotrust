package serviceaccount

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/pkg/middleware"
)

type nonFlushingResponseWriter struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (w *nonFlushingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *nonFlushingResponseWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *nonFlushingResponseWriter) WriteHeader(statusCode int) {
	w.code = statusCode
}

func newEventTestKeyStore(t *testing.T) *auth.KeyStore {
	t.Helper()
	ks, err := auth.LoadOrGenerateKeyStore("", "", auth.AlgEdDSA)
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

func TestEventsRejectsMissingOrInvalidToken(t *testing.T) {
	t.Run("missing token", func(t *testing.T) {
		h := NewHandler(nil, NewEventHub(), newEventTestKeyStore(t), nil)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts/events", nil)
		rr := httptest.NewRecorder()

		h.Events(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		h := NewHandler(nil, NewEventHub(), newEventTestKeyStore(t), nil)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts/events", nil)
		req.Header.Set("Authorization", "Bearer invalid.token")
		rr := httptest.NewRecorder()

		h.Events(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
		}
	})
}

func TestEventsRejectsTokenWithoutPermission(t *testing.T) {
	ks := newEventTestKeyStore(t)
	pair, err := auth.GenerateTokenPair(ks, "user-1", "admin@example.com", "en", []string{"admin"}, []string{"users:read"}, time.Minute)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	h := NewHandler(nil, NewEventHub(), ks, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts/events", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()

	h.Events(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}

func TestEventsRejectsUnsupportedStreaming(t *testing.T) {
	ks := newEventTestKeyStore(t)
	token := newServiceAccountReadToken(t, ks)
	h := NewHandler(nil, NewEventHub(), ks, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rw := &nonFlushingResponseWriter{}

	h.Events(rw, req)

	if rw.code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d body=%s", rw.code, http.StatusInternalServerError, rw.body.String())
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
	listErr        error
	revokeErr      error
	setActiveErr   error
	rotateErr      error
	createdScopes  []string
	createdName    string
	updatedName    string
	updatedScopes  []string
	updatedActive  bool
	updatedID      string
	updatedExpiry  *time.Time
	setActiveValue bool
	listResult     ListResult
	listParams     ListParams
	revokeID       string
}

func (s *fakeServiceAccountService) Create(ctx context.Context, name, createdBy string, caller *auth.Claims, scopes []string, expiresAt *time.Time) (*ServiceAccount, string, error) {
	s.createdName = name
	s.createdScopes = scopes
	if s.createErr != nil {
		return nil, "", s.createErr
	}
	return &ServiceAccount{ID: "sa1", Name: name, ClientID: "svc_1", IsActive: true, Scopes: scopes, CreatedAt: time.Now()}, "secret", nil
}

func (s *fakeServiceAccountService) Update(ctx context.Context, id, name string, caller *auth.Claims, scopes []string, expiresAt *time.Time, active bool) (*ServiceAccount, error) {
	s.updatedID = id
	s.updatedName = name
	s.updatedScopes = scopes
	s.updatedActive = active
	s.updatedExpiry = expiresAt
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return &ServiceAccount{ID: id, Name: name, ClientID: "svc_1", IsActive: active, Scopes: scopes, CreatedAt: time.Now()}, nil
}

func (s *fakeServiceAccountService) List(ctx context.Context, p ListParams) (ListResult, error) {
	s.listParams = p
	if s.listErr != nil {
		return ListResult{}, s.listErr
	}
	return s.listResult, nil
}

func (s *fakeServiceAccountService) Revoke(ctx context.Context, id string) error {
	s.revokeID = id
	return s.revokeErr
}

func (s *fakeServiceAccountService) SetActive(ctx context.Context, id string, active bool) error {
	s.setActiveValue = active
	return s.setActiveErr
}

func (s *fakeServiceAccountService) RotateSecret(ctx context.Context, id string) (*ServiceAccount, string, error) {
	if s.rotateErr != nil {
		return nil, "", s.rotateErr
	}
	return &ServiceAccount{ID: id, Name: "rotated", ClientID: "svc_1", IsActive: true, CreatedAt: time.Now()}, "newsecret", nil
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

func TestCreateValidationAndSuccess(t *testing.T) {
	t.Run("invalid request", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{}, hub: NewEventHub()}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-accounts", bytes.NewReader([]byte(`{"scopes":["users:read"]}`)))
		rr := httptest.NewRecorder()
		h.Create(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	})

	t.Run("invalid expires at", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{}, hub: NewEventHub()}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-accounts", bytes.NewReader([]byte(`{"name":"bot","expires_at":"2026/01/01"}`)))
		rr := httptest.NewRecorder()
		h.Create(rr, requestWithClaims(req))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	})

	t.Run("name taken", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{createErr: ErrNameTaken}, hub: NewEventHub()}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-accounts", bytes.NewReader([]byte(`{"name":"bot"}`)))
		rr := httptest.NewRecorder()
		h.Create(rr, requestWithClaims(req))
		if rr.Code != http.StatusConflict {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusConflict, rr.Body.String())
		}
	})

	t.Run("success", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{}, hub: NewEventHub()}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-accounts", bytes.NewReader([]byte(`{"name":"bot","scopes":["users:read"]}`)))
		rr := httptest.NewRecorder()
		h.Create(rr, requestWithClaims(req))
		if rr.Code != http.StatusCreated {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusCreated, rr.Body.String())
		}
		if !bytes.Contains(rr.Body.Bytes(), []byte(`"client_secret":"secret"`)) {
			t.Fatalf("expected secret in response, got %s", rr.Body.String())
		}
	})
}

func TestListHandler(t *testing.T) {
	t.Run("internal error", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{listErr: errors.New("boom")}, hub: NewEventHub()}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts", nil)
		rr := httptest.NewRecorder()
		h.List(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusInternalServerError, rr.Body.String())
		}
	})

	t.Run("success", func(t *testing.T) {
		svc := &fakeServiceAccountService{listResult: ListResult{Accounts: []*ServiceAccount{{ID: "sa1", Name: "bot", ClientID: "svc_1", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}}, Total: 1}}
		h := &Handler{svc: svc, hub: NewEventHub()}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/service-accounts?limit=5&offset=2&sort_by=name&sort_dir=desc&name=bot&status=active", nil)
		rr := httptest.NewRecorder()
		h.List(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
		var payload pagedSAResponse
		if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Total != 1 || len(payload.Data) != 1 {
			t.Fatalf("payload=%+v", payload)
		}
		if svc.listParams.Limit != 5 || svc.listParams.Offset != 2 || svc.listParams.SortBy != "name" || svc.listParams.SortDir != "desc" || svc.listParams.Name != "bot" || svc.listParams.Status != "active" {
			t.Fatalf("unexpected list params: %+v", svc.listParams)
		}
	})
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

func TestSetStatusValidationAndInternalError(t *testing.T) {
	t.Run("invalid request", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{}, hub: NewEventHub()}
		req := requestWithURLParam(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/service-accounts/sa1/status", bytes.NewReader([]byte(`{bad`))), "id", "sa1")
		rr := httptest.NewRecorder()
		h.SetStatus(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	})

	t.Run("internal error", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{setActiveErr: errors.New("boom")}, hub: NewEventHub()}
		req := requestWithURLParam(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/service-accounts/sa1/status", bytes.NewReader([]byte(`{"is_active":true}`))), "id", "sa1")
		rr := httptest.NewRecorder()
		h.SetStatus(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusInternalServerError, rr.Body.String())
		}
	})
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

func TestUpdateValidationAndSuccess(t *testing.T) {
	t.Run("invalid request body", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{}, hub: NewEventHub()}
		req := requestWithURLParam(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/service-accounts/sa1", bytes.NewReader([]byte(`{bad`))), "id", "sa1")
		rr := httptest.NewRecorder()

		h.Update(rr, requestWithClaims(req))

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	})

	t.Run("invalid expires at", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{}, hub: NewEventHub()}
		req := requestWithURLParam(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/service-accounts/sa1", bytes.NewReader([]byte(`{"name":"bot","expires_at":"2026/01/01","is_active":true}`))), "id", "sa1")
		rr := httptest.NewRecorder()

		h.Update(rr, requestWithClaims(req))

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	})

	t.Run("name taken", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{updateErr: ErrNameTaken}, hub: NewEventHub()}
		req := requestWithURLParam(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/service-accounts/sa1", bytes.NewReader([]byte(`{"name":"bot","is_active":true}`))), "id", "sa1")
		rr := httptest.NewRecorder()

		h.Update(rr, requestWithClaims(req))

		if rr.Code != http.StatusConflict {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusConflict, rr.Body.String())
		}
	})

	t.Run("success", func(t *testing.T) {
		svc := &fakeServiceAccountService{}
		h := &Handler{svc: svc, hub: NewEventHub()}
		req := requestWithURLParam(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/service-accounts/sa1", bytes.NewReader([]byte(`{"name":"bot-updated","scopes":["users:read"],"expires_at":"2026-01-09","is_active":true}`))), "id", "sa1")
		rr := httptest.NewRecorder()

		h.Update(rr, requestWithClaims(req))

		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
		if svc.updatedID != "sa1" || svc.updatedName != "bot-updated" || !svc.updatedActive {
			t.Fatalf("unexpected update args: id=%q name=%q active=%v", svc.updatedID, svc.updatedName, svc.updatedActive)
		}
		if len(svc.updatedScopes) != 1 || svc.updatedScopes[0] != "users:read" {
			t.Fatalf("unexpected update scopes: %v", svc.updatedScopes)
		}
		if svc.updatedExpiry == nil || svc.updatedExpiry.Format("2006-01-02T15:04:05Z") != "2026-01-09T23:59:59Z" {
			t.Fatalf("unexpected update expiry: %v", svc.updatedExpiry)
		}
	})
}

func TestRevokeSuccess(t *testing.T) {
	svc := &fakeServiceAccountService{}
	h := &Handler{svc: svc, hub: NewEventHub()}
	req := requestWithURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/service-accounts/sa1", nil), "id", "sa1")
	rr := httptest.NewRecorder()

	h.Revoke(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
	if svc.revokeID != "sa1" {
		t.Fatalf("revoke id=%q want=sa1", svc.revokeID)
	}
}

func TestRotateSecretSuccess(t *testing.T) {
	svc := &fakeServiceAccountService{}
	h := &Handler{svc: svc, hub: NewEventHub()}
	req := requestWithURLParam(
		httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-accounts/sa1/rotate", nil),
		"id",
		"sa1",
	)
	rr := httptest.NewRecorder()

	h.RotateSecret(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"client_secret":"newsecret"`)) {
		t.Fatalf("expected response to contain rotated secret, got %s", rr.Body.String())
	}
}

func TestRevokeAndRotateInternalErrors(t *testing.T) {
	t.Run("revoke internal", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{revokeErr: errors.New("boom")}, hub: NewEventHub()}
		req := requestWithURLParam(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/service-accounts/sa1", nil), "id", "sa1")
		rr := httptest.NewRecorder()
		h.Revoke(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusInternalServerError, rr.Body.String())
		}
	})

	t.Run("rotate not found", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{rotateErr: ErrNotFound}, hub: NewEventHub()}
		req := requestWithURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-accounts/sa1/rotate", nil), "id", "sa1")
		rr := httptest.NewRecorder()
		h.RotateSecret(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNotFound, rr.Body.String())
		}
	})

	t.Run("rotate internal", func(t *testing.T) {
		h := &Handler{svc: &fakeServiceAccountService{rotateErr: errors.New("boom")}, hub: NewEventHub()}
		req := requestWithURLParam(httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-accounts/sa1/rotate", nil), "id", "sa1")
		rr := httptest.NewRecorder()
		h.RotateSecret(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusInternalServerError, rr.Body.String())
		}
	})
}

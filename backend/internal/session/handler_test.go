package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/zerotrust/backend/internal/auth"
	authmw "github.com/zerotrust/backend/pkg/middleware"
)

type fakeStore struct {
	revokeByIDErr      error
	revokeOthersErr    error
	revokeByIDCalled   bool
	revokeOthersCalled bool
	revokeByIDID       string
	revokeByIDUserID   string
	revokeOthersUserID string
	revokeOthersHash   string
}

func (s *fakeStore) ListForUser(ctx context.Context, userID, currentHash string) ([]SessionInfo, error) {
	return nil, nil
}

func (s *fakeStore) RevokeByID(ctx context.Context, id, userID string) error {
	s.revokeByIDCalled = true
	s.revokeByIDID = id
	s.revokeByIDUserID = userID
	return s.revokeByIDErr
}

func (s *fakeStore) RevokeOtherSessions(ctx context.Context, userID, currentHash string) error {
	s.revokeOthersCalled = true
	s.revokeOthersUserID = userID
	s.revokeOthersHash = currentHash
	return s.revokeOthersErr
}

func withClaims(r *http.Request, userID string) *http.Request {
	claims := &auth.Claims{UserID: userID}
	ctx := context.WithValue(r.Context(), authmw.ClaimsKey, claims)
	return r.WithContext(ctx)
}

func withRouteID(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	return r.WithContext(ctx)
}

func TestRevoke_ByID_Success(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(store, NewEventHub())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/s1", nil)
	req = withClaims(req, "u1")
	req = withRouteID(req, "s1")
	rr := httptest.NewRecorder()

	h.Revoke(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
	if !store.revokeByIDCalled {
		t.Fatal("expected RevokeByID to be called")
	}
	if store.revokeByIDID != "s1" || store.revokeByIDUserID != "u1" {
		t.Fatalf("unexpected revoke args id=%q user=%q", store.revokeByIDID, store.revokeByIDUserID)
	}
}

func TestRevoke_ByID_NotFound(t *testing.T) {
	store := &fakeStore{revokeByIDErr: ErrNotFound}
	h := NewHandler(store, NewEventHub())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions/missing", nil)
	req = withClaims(req, "u1")
	req = withRouteID(req, "missing")
	rr := httptest.NewRecorder()

	h.Revoke(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}

func TestRevokeOthers_Success_WithRefreshCookie(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(store, NewEventHub())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions", nil)
	req = withClaims(req, "u1")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh-token"})
	rr := httptest.NewRecorder()

	h.RevokeOthers(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
	if !store.revokeOthersCalled {
		t.Fatal("expected RevokeOtherSessions to be called")
	}
	if store.revokeOthersUserID != "u1" {
		t.Fatalf("unexpected user id %q", store.revokeOthersUserID)
	}
	if store.revokeOthersHash == "" {
		t.Fatal("expected hashed refresh token to be passed")
	}
	if store.revokeOthersHash == "raw-refresh-token" {
		t.Fatal("expected refresh token to be hashed before passing to store")
	}
}

func TestRevokeOthers_FailsWithoutRefreshCookie(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(store, NewEventHub())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions", nil)
	req = withClaims(req, "u1")
	rr := httptest.NewRecorder()

	h.RevokeOthers(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if store.revokeOthersCalled {
		t.Fatal("store should not be called when refresh cookie is missing")
	}
}

func TestRevokeOthers_StoreError(t *testing.T) {
	store := &fakeStore{revokeOthersErr: errors.New("db down")}
	h := NewHandler(store, NewEventHub())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/sessions", nil)
	req = withClaims(req, "u1")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh-token"})
	rr := httptest.NewRecorder()

	h.RevokeOthers(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusInternalServerError, rr.Body.String())
	}
}

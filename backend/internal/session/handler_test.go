package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zerotrust/backend/internal/auth"
	authmw "github.com/zerotrust/backend/pkg/middleware"
)

type fakeStore struct {
	listForUserResp    []SessionInfo
	listForUserErr     error
	listForUserCalled  bool
	listForUserUserID  string
	listForUserHash    string
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
	s.listForUserCalled = true
	s.listForUserUserID = userID
	s.listForUserHash = currentHash
	if s.listForUserErr != nil {
		return nil, s.listForUserErr
	}
	return s.listForUserResp, nil
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

func TestList_Success(t *testing.T) {
	store := &fakeStore{listForUserResp: []SessionInfo{{ID: "s1"}}}
	h := NewHandler(store, NewEventHub())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req = withClaims(req, "u1")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh-token"})
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !store.listForUserCalled {
		t.Fatal("expected ListForUser to be called")
	}
	if store.listForUserUserID != "u1" {
		t.Fatalf("unexpected user id %q", store.listForUserUserID)
	}
	if store.listForUserHash == "" || store.listForUserHash == "raw-refresh-token" {
		t.Fatalf("expected hashed refresh token, got %q", store.listForUserHash)
	}

	var payload []SessionInfo
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload) != 1 || payload[0].ID != "s1" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestList_StoreError(t *testing.T) {
	store := &fakeStore{listForUserErr: errors.New("db down")}
	h := NewHandler(store, NewEventHub())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req = withClaims(req, "u1")
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusInternalServerError, rr.Body.String())
	}
}

func TestList_RequiresAuth(t *testing.T) {
	h := NewHandler(&fakeStore{}, NewEventHub())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
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

type noFlushWriter struct {
	h http.Header
	b strings.Builder
	s int
}

func (w *noFlushWriter) Header() http.Header {
	if w.h == nil {
		w.h = make(http.Header)
	}
	return w.h
}

func (w *noFlushWriter) Write(p []byte) (int, error) {
	return w.b.Write(p)
}

func (w *noFlushWriter) WriteHeader(statusCode int) {
	w.s = statusCode
}

func TestEvents_StreamingUnsupported(t *testing.T) {
	h := NewHandler(&fakeStore{}, NewEventHub())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/events", nil)
	req = withClaims(req, "u1")

	w := &noFlushWriter{}
	h.Events(w, req)

	if w.s != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d body=%s", w.s, http.StatusInternalServerError, w.b.String())
	}
}

func waitForSubscription(t *testing.T, hub *EventHub, userID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		hub.mu.Lock()
		count := len(hub.clients[userID])
		hub.mu.Unlock()
		if count > 0 {
			return
		}
	}
	t.Fatalf("timed out waiting for subscription for user %q", userID)
}

func runEventStream(t *testing.T, req *http.Request, hub *EventHub, drive func(rec *httptest.ResponseRecorder, cancel context.CancelFunc)) string {
	t.Helper()
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h := NewHandler(&fakeStore{}, hub)

	done := make(chan struct{})
	go func() {
		h.Events(rr, req)
		close(done)
	}()

	waitForSubscription(t, hub, "u1")
	drive(rr, cancel)
	<-done
	return rr.Body.String()
}

func TestEvents_RevokedCurrentSessionEndsStream(t *testing.T) {
	hub := NewEventHub()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/events", nil)
	req = withClaims(req, "u1")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh-token"})
	body := runEventStream(t, req, hub, func(_ *httptest.ResponseRecorder, _ context.CancelFunc) {
		hub.BroadcastRevoked("u1", hashRefreshToken("raw-refresh-token"))
	})

	if !strings.Contains(body, "data: revoked") {
		t.Fatalf("unexpected stream body: %q", body)
	}
}

func TestEvents_RevokedOtherSessionSendsChange(t *testing.T) {
	hub := NewEventHub()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/events", nil)
	req = withClaims(req, "u1")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh-token"})
	body := runEventStream(t, req, hub, func(rr *httptest.ResponseRecorder, cancel context.CancelFunc) {
		hub.BroadcastRevoked("u1", "someone-else")
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(rr.Body.String(), "data: change") {
				cancel()
				return
			}
		}
		cancel()
	})

	if !strings.Contains(body, "data: change") {
		t.Fatalf("unexpected stream body: %q", body)
	}
}

func TestEvents_RevokedOthersForKeeperSendsChange(t *testing.T) {
	hub := NewEventHub()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/events", nil)
	req = withClaims(req, "u1")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh-token"})
	body := runEventStream(t, req, hub, func(rr *httptest.ResponseRecorder, cancel context.CancelFunc) {
		hub.BroadcastRevokedOthers("u1", hashRefreshToken("raw-refresh-token"))
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(rr.Body.String(), "data: change") {
				cancel()
				return
			}
		}
		cancel()
	})

	if !strings.Contains(body, "data: change") {
		t.Fatalf("unexpected stream body: %q", body)
	}
}

func TestEvents_RevokedOthersForRevokedSessionEndsStream(t *testing.T) {
	hub := NewEventHub()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/events", nil)
	req = withClaims(req, "u1")
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "raw-refresh-token"})
	body := runEventStream(t, req, hub, func(_ *httptest.ResponseRecorder, _ context.CancelFunc) {
		hub.BroadcastRevokedOthers("u1", "different-session")
	})

	if !strings.Contains(body, "data: revoked") {
		t.Fatalf("unexpected stream body: %q", body)
	}
}

func TestEvents_DefaultEventSendsChange(t *testing.T) {
	hub := NewEventHub()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/events", nil)
	req = withClaims(req, "u1")
	body := runEventStream(t, req, hub, func(rr *httptest.ResponseRecorder, cancel context.CancelFunc) {
		hub.Broadcast("u1")
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(rr.Body.String(), "data: change") {
				cancel()
				return
			}
		}
		cancel()
	})

	if !strings.Contains(body, "data: connected") || !strings.Contains(body, "data: change") {
		t.Fatalf("unexpected stream body: %q", body)
	}
}

func TestEvents_RequiresAuth(t *testing.T) {
	h := NewHandler(&fakeStore{}, NewEventHub())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/events", nil)
	rr := httptest.NewRecorder()

	h.Events(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
}

func TestEvents_RevokedAllEndsStream(t *testing.T) {
	hub := NewEventHub()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/events", nil)
	req = withClaims(req, "u1")
	body := runEventStream(t, req, hub, func(_ *httptest.ResponseRecorder, _ context.CancelFunc) {
		hub.BroadcastRevokedAll("u1")
	})

	if !strings.Contains(body, "data: connected") || !strings.Contains(body, "data: revoked") {
		t.Fatalf("unexpected stream body: %q", body)
	}
}

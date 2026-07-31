package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/middleware"
)

// reqWithClaims builds a request carrying a chi {id} param and the caller's
// claims, so the admin guards (ISSUE_LIST #34) can be tested without a database.
func reqWithClaims(method, body, targetID, callerID string) *http.Request {
	r := httptest.NewRequest(method, "/admin/users/"+targetID, bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", targetID)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, middleware.ClaimsKey, &auth.Claims{UserID: callerID})
	return r.WithContext(ctx)
}

func TestUpdateRoles_SelfModificationForbidden(t *testing.T) {
	m := &mockUserManager{}
	h := NewHandler(m, nil, nil, nil)

	w := httptest.NewRecorder()
	h.UpdateRoles(w, reqWithClaims(http.MethodPatch, `{"roles":["viewer"]}`, "admin-1", "admin-1"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if m.lastSetRolesID != "" {
		t.Fatal("SetRoles must not be called when modifying self")
	}
}

func TestSetStatus_SelfModificationForbidden(t *testing.T) {
	m := &mockUserManager{}
	h := NewHandler(m, nil, nil, nil)

	w := httptest.NewRecorder()
	h.SetStatus(w, reqWithClaims(http.MethodPatch, `{"is_active":false}`, "admin-1", "admin-1"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if m.lastSetActiveID != "" {
		t.Fatal("SetActive must not be called when modifying self")
	}
}

func TestUpdateRoles_LastAdminConflict(t *testing.T) {
	m := &mockUserManager{setRolesErr: user.ErrLastAdmin}
	h := NewHandler(m, nil, nil, nil)

	w := httptest.NewRecorder()
	h.UpdateRoles(w, reqWithClaims(http.MethodPatch, `{"roles":["viewer"]}`, "admin-2", "admin-1"))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
	if m.lastSetRolesID != "admin-2" {
		t.Fatalf("SetRoles should be called for a different user, got %q", m.lastSetRolesID)
	}
}

func TestSetStatus_LastAdminConflict(t *testing.T) {
	m := &mockUserManager{setActiveErr: user.ErrLastAdmin}
	h := NewHandler(m, nil, nil, nil)

	w := httptest.NewRecorder()
	h.SetStatus(w, reqWithClaims(http.MethodPatch, `{"is_active":false}`, "admin-2", "admin-1"))

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

// reqWithClaimsRoles is reqWithClaims plus a caller role set, for testing the
// role-escalation guard (ISSUE_LIST #83).
func reqWithClaimsRoles(method, body, targetID, callerID string, callerRoles ...string) *http.Request {
	r := httptest.NewRequest(method, "/admin/users/"+targetID, bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", targetID)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	ctx = context.WithValue(ctx, middleware.ClaimsKey, &auth.Claims{UserID: callerID, Roles: callerRoles})
	return r.WithContext(ctx)
}

func TestUpdateRoles_EscalationForbiddenWithoutAdminClaim(t *testing.T) {
	m := &mockUserManager{}
	h := NewHandler(m, nil, nil, nil)

	w := httptest.NewRecorder()
	h.UpdateRoles(w, reqWithClaimsRoles(http.MethodPatch, `{"roles":["admin"]}`, "user-9", "svc-1", "users:update"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if m.lastSetRolesID != "" {
		t.Fatal("SetRoles must not be called when caller cannot grant admin")
	}
}

func TestUpdateRoles_AdminCanGrantAdmin(t *testing.T) {
	m := &mockUserManager{}
	h := NewHandler(m, nil, nil, nil)

	w := httptest.NewRecorder()
	h.UpdateRoles(w, reqWithClaimsRoles(http.MethodPatch, `{"roles":["admin"]}`, "user-9", "admin-1", "admin"))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if m.lastSetRolesID != "user-9" {
		t.Fatalf("SetRoles should be called for user-9, got %q", m.lastSetRolesID)
	}
}

func TestUpdateRoles_OtherUserSucceeds(t *testing.T) {
	m := &mockUserManager{}
	h := NewHandler(m, nil, nil, nil)

	w := httptest.NewRecorder()
	h.UpdateRoles(w, reqWithClaims(http.MethodPatch, `{"roles":["viewer"]}`, "user-9", "admin-1"))

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if m.lastSetRolesID != "user-9" {
		t.Fatalf("SetRoles should be called for user-9, got %q", m.lastSetRolesID)
	}
}

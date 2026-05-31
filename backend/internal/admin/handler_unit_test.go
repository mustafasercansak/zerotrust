package admin

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
	"github.com/zerotrust/backend/internal/session"
	"github.com/zerotrust/backend/internal/user"
)

func TestQueryInt(t *testing.T) {
	tests := []struct {
		name string
		in   string
		def  int
		want int
	}{
		{name: "valid", in: "10", def: 25, want: 10},
		{name: "negative", in: "-1", def: 25, want: 25},
		{name: "invalid", in: "abc", def: 25, want: 25},
		{name: "empty", in: "", def: 25, want: 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryInt(tt.in, tt.def)
			if got != tt.want {
				t.Fatalf("queryInt(%q, %d)=%d; want %d", tt.in, tt.def, got, tt.want)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()

	writeError(rr, http.StatusBadRequest, "invalid_request")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type=%q want application/json", ct)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "invalid_request" {
		t.Fatalf("error code=%q want=invalid_request", body["error"])
	}
}

func TestToResponseHandlesNilRoles(t *testing.T) {
	u := &user.User{
		ID:        "u1",
		Email:     "test@example.com",
		FirstName: "Test",
		LastName:  "User",
		HasAvatar: true,
		Locale:    "en",
		IsActive:  true,
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Roles:     nil,
	}

	resp := toResponse(u, 3)

	if resp.CreatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("created_at=%q", resp.CreatedAt)
	}
	if resp.ActiveSessions != 3 {
		t.Fatalf("active_sessions=%d want=3", resp.ActiveSessions)
	}
	if resp.Roles == nil {
		t.Fatal("roles should be an empty slice, got nil")
	}
	if len(resp.Roles) != 0 {
		t.Fatalf("roles len=%d want=0", len(resp.Roles))
	}
}

type mockUserManager struct {
	listResult         user.ListResult
	listErr            error
	registerUser       *user.User
	registerErr        error
	updateProfileUser  *user.User
	updateProfileErr   error
	setRolesErr        error
	setActiveErr       error
	lastListParams     user.ListParams
	lastSetRolesID     string
	lastSetRoles       []string
	lastSetActiveID    string
	lastSetActiveValue bool
}

func (m *mockUserManager) List(_ context.Context, p user.ListParams) (user.ListResult, error) {
	m.lastListParams = p
	if m.listErr != nil {
		return user.ListResult{}, m.listErr
	}
	return m.listResult, nil
}

func (m *mockUserManager) RegisterWithRoles(_ context.Context, email, password, locale string, roles []string) (*user.User, error) {
	if m.registerErr != nil {
		return nil, m.registerErr
	}
	return m.registerUser, nil
}

func (m *mockUserManager) UpdateProfile(_ context.Context, userID, firstName, lastName string) (*user.User, error) {
	if m.updateProfileErr != nil {
		return nil, m.updateProfileErr
	}
	return m.updateProfileUser, nil
}

func (m *mockUserManager) SetRoles(_ context.Context, userID string, roles []string) error {
	m.lastSetRolesID = userID
	m.lastSetRoles = append([]string(nil), roles...)
	return m.setRolesErr
}

func (m *mockUserManager) SetActive(_ context.Context, userID string, active bool) error {
	m.lastSetActiveID = userID
	m.lastSetActiveValue = active
	return m.setActiveErr
}

type mockSessionManagerUnit struct {
	listResp         []session.SessionInfo
	listErr          error
	revokeAllErr     error
	revokeByIDErr    error
	revokeAllCalls   int
	revokeByIDCalls  int
	lastListUserID   string
	lastRevokeAllID  string
	lastRevokeID     string
	lastRevokeUserID string
}

func (m *mockSessionManagerUnit) ListForUser(_ context.Context, userID, currentHash string) ([]session.SessionInfo, error) {
	m.lastListUserID = userID
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResp, nil
}

func (m *mockSessionManagerUnit) RevokeAllForUser(_ context.Context, userID string) error {
	m.revokeAllCalls++
	m.lastRevokeAllID = userID
	if m.revokeAllErr != nil {
		return m.revokeAllErr
	}
	return nil
}

func (m *mockSessionManagerUnit) RevokeByID(_ context.Context, id, userID string) error {
	m.revokeByIDCalls++
	m.lastRevokeID = id
	m.lastRevokeUserID = userID
	if m.revokeByIDErr != nil {
		return m.revokeByIDErr
	}
	return nil
}

func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx, _ := r.Context().Value(chi.RouteCtxKey).(*chi.Context)
	if rctx == nil {
		rctx = chi.NewRouteContext()
	}
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestListUsersWithMockService(t *testing.T) {
	u := &user.User{
		ID:        "u1",
		Email:     "one@example.com",
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	mgr := &mockUserManager{
		listResult: user.ListResult{
			Users:          []*user.User{u},
			ActiveSessions: map[string]int{"u1": 4},
			Total:          1,
		},
	}
	h := NewHandler(mgr, nil)

	req, _ := http.NewRequest("GET", "/api/v1/admin/users?limit=9&offset=2&sort_by=email&sort_dir=asc&email=one&status=active", nil)
	rr := httptest.NewRecorder()

	h.ListUsers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
	}
	if mgr.lastListParams.Limit != 9 || mgr.lastListParams.Offset != 2 {
		t.Fatalf("unexpected pagination params: %+v", mgr.lastListParams)
	}
	if mgr.lastListParams.SortBy != "email" || mgr.lastListParams.SortDir != "asc" {
		t.Fatalf("unexpected sort params: %+v", mgr.lastListParams)
	}

	var payload pagedResponse[userResponse]
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Total != 1 || len(payload.Data) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Data[0].ActiveSessions != 4 {
		t.Fatalf("active sessions=%d want=4", payload.Data[0].ActiveSessions)
	}
}

func TestCreateUserWithMockService_ErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
	}{
		{name: "email taken", err: user.ErrEmailTaken, statusCode: http.StatusConflict},
		{name: "unknown role", err: user.ErrUnknownRole, statusCode: http.StatusUnprocessableEntity},
		{name: "internal", err: errors.New("boom"), statusCode: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &mockUserManager{registerErr: tt.err}
			h := NewHandler(mgr, nil)

			body := `{"email":"mock@example.com","password":"Password1!","roles":["admin"]}`
			req, _ := http.NewRequest("POST", "/api/v1/admin/users", bytes.NewBufferString(body))
			rr := httptest.NewRecorder()

			h.CreateUser(rr, req)

			if rr.Code != tt.statusCode {
				t.Fatalf("status=%d want=%d", rr.Code, tt.statusCode)
			}
		})
	}
}

func TestSetStatusWithMockService_RevokesOnDeactivate(t *testing.T) {
	mgr := &mockUserManager{}
	sessions := &mockSessionManagerUnit{}
	h := NewHandler(mgr, sessions)

	req, _ := http.NewRequest("PATCH", "/api/v1/admin/users/u1/status", bytes.NewBufferString(`{"is_active":false}`))
	req = withURLParam(req, "id", "u1")
	rr := httptest.NewRecorder()

	h.SetStatus(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusNoContent)
	}
	if mgr.lastSetActiveID != "u1" || mgr.lastSetActiveValue != false {
		t.Fatalf("unexpected setActive call: id=%q active=%v", mgr.lastSetActiveID, mgr.lastSetActiveValue)
	}
	if sessions.revokeAllCalls != 1 {
		t.Fatalf("revoke calls=%d want=1", sessions.revokeAllCalls)
	}
}

func TestSetStatusWithMockService_NotFound(t *testing.T) {
	mgr := &mockUserManager{setActiveErr: user.ErrNotFound}
	h := NewHandler(mgr, nil)

	req, _ := http.NewRequest("PATCH", "/api/v1/admin/users/u1/status", bytes.NewBufferString(`{"is_active":true}`))
	req = withURLParam(req, "id", "u1")
	rr := httptest.NewRecorder()

	h.SetStatus(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusNotFound)
	}
}

func TestUpdateRolesWithMockService(t *testing.T) {
	t.Run("invalid request", func(t *testing.T) {
		h := NewHandler(&mockUserManager{}, nil)
		req, _ := http.NewRequest("PATCH", "/api/v1/admin/users/u1/roles", bytes.NewBufferString("{bad"))
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.UpdateRoles(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("unknown role", func(t *testing.T) {
		mgr := &mockUserManager{setRolesErr: user.ErrUnknownRole}
		h := NewHandler(mgr, nil)
		req, _ := http.NewRequest("PATCH", "/api/v1/admin/users/u1/roles", bytes.NewBufferString(`{"roles":["admin"]}`))
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.UpdateRoles(rr, req)
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusUnprocessableEntity)
		}
	})

	t.Run("success", func(t *testing.T) {
		mgr := &mockUserManager{}
		h := NewHandler(mgr, nil)
		req, _ := http.NewRequest("PATCH", "/api/v1/admin/users/u1/roles", bytes.NewBufferString(`{"roles":["admin","viewer"]}`))
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.UpdateRoles(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusNoContent)
		}
		if mgr.lastSetRolesID != "u1" || len(mgr.lastSetRoles) != 2 {
			t.Fatalf("unexpected set roles call id=%q roles=%v", mgr.lastSetRolesID, mgr.lastSetRoles)
		}
	})
}

func TestAdminSessionEndpointsWithMockService(t *testing.T) {
	t.Run("list sessions unavailable", func(t *testing.T) {
		h := NewHandler(&mockUserManager{}, nil)
		req, _ := http.NewRequest("GET", "/api/v1/admin/users/u1/sessions", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.ListUserSessions(rr, req)
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("list sessions success", func(t *testing.T) {
		sessions := &mockSessionManagerUnit{listResp: []session.SessionInfo{{ID: "s1"}}}
		h := NewHandler(&mockUserManager{}, sessions)
		req, _ := http.NewRequest("GET", "/api/v1/admin/users/u1/sessions", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.ListUserSessions(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
		}
		if sessions.lastListUserID != "u1" {
			t.Fatalf("lastListUserID=%q want=u1", sessions.lastListUserID)
		}
	})

	t.Run("list sessions error", func(t *testing.T) {
		sessions := &mockSessionManagerUnit{listErr: errors.New("boom")}
		h := NewHandler(&mockUserManager{}, sessions)
		req, _ := http.NewRequest("GET", "/api/v1/admin/users/u1/sessions", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.ListUserSessions(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusInternalServerError)
		}
	})

	t.Run("revoke all success", func(t *testing.T) {
		sessions := &mockSessionManagerUnit{}
		h := NewHandler(&mockUserManager{}, sessions)
		req, _ := http.NewRequest("DELETE", "/api/v1/admin/users/u1/sessions", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.RevokeAllUserSessions(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusNoContent)
		}
		if sessions.lastRevokeAllID != "u1" {
			t.Fatalf("lastRevokeAllID=%q want=u1", sessions.lastRevokeAllID)
		}
	})

	t.Run("revoke all error", func(t *testing.T) {
		sessions := &mockSessionManagerUnit{revokeAllErr: errors.New("boom")}
		h := NewHandler(&mockUserManager{}, sessions)
		req, _ := http.NewRequest("DELETE", "/api/v1/admin/users/u1/sessions", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.RevokeAllUserSessions(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusInternalServerError)
		}
	})

	t.Run("revoke one success", func(t *testing.T) {
		sessions := &mockSessionManagerUnit{}
		h := NewHandler(&mockUserManager{}, sessions)
		req, _ := http.NewRequest("DELETE", "/api/v1/admin/users/u1/sessions/s1", nil)
		req = withURLParam(req, "id", "u1")
		req = withURLParam(req, "sessionId", "s1")
		rr := httptest.NewRecorder()
		h.RevokeUserSession(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusNoContent)
		}
		if sessions.lastRevokeID != "s1" || sessions.lastRevokeUserID != "u1" {
			t.Fatalf("unexpected revoke call id=%q user=%q", sessions.lastRevokeID, sessions.lastRevokeUserID)
		}
	})

	t.Run("revoke one error", func(t *testing.T) {
		sessions := &mockSessionManagerUnit{revokeByIDErr: errors.New("boom")}
		h := NewHandler(&mockUserManager{}, sessions)
		req, _ := http.NewRequest("DELETE", "/api/v1/admin/users/u1/sessions/s1", nil)
		req = withURLParam(req, "id", "u1")
		req = withURLParam(req, "sessionId", "s1")
		rr := httptest.NewRecorder()
		h.RevokeUserSession(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusInternalServerError)
		}
	})
}

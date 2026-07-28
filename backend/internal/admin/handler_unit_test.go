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
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/session"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/internal/webauthn"
	middleware "github.com/zerotrust/backend/pkg/middleware"
)

func withClaims(r *http.Request, userID string) *http.Request {
	claims := &auth.Claims{UserID: userID}
	return r.WithContext(context.WithValue(r.Context(), middleware.ClaimsKey, claims))
}

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
		UpdatedAt: time.Date(2026, 6, 7, 8, 9, 10, 0, time.UTC),
		Roles:     nil,
	}

	resp := toResponse(u, 3, true, 2)

	if resp.CreatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("created_at=%q", resp.CreatedAt)
	}
	if resp.UpdatedAt != "2026-06-07T08:09:10Z" {
		t.Fatalf("updated_at=%q", resp.UpdatedAt)
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
	listResult          user.ListResult
	listErr             error
	registerUser        *user.User
	registerErr         error
	updateProfileUser   *user.User
	updateProfileErr    error
	setRolesErr         error
	setActiveErr        error
	bulkSetActiveErr    error
	bulkSetActiveIDs    []string
	bulkSetActiveValue  bool
	findByIDUser        *user.User
	findByIDErr         error
	lastListParams      user.ListParams
	lastSetRolesID      string
	lastSetRoles        []string
	lastSetActiveID     string
	lastSetActiveValue  bool
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

func (m *mockUserManager) BulkSetActive(_ context.Context, userIDs []string, active bool) error {
	m.bulkSetActiveIDs = userIDs
	m.bulkSetActiveValue = active
	return m.bulkSetActiveErr
}

func (m *mockUserManager) FindByID(_ context.Context, id string) (*user.User, error) {
	if m.findByIDErr != nil {
		return nil, m.findByIDErr
	}
	return m.findByIDUser, nil
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
		UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	mgr := &mockUserManager{
		listResult: user.ListResult{
			Users:          []*user.User{u},
			ActiveSessions: map[string]int{"u1": 4},
			Total:          1,
		},
	}
	h := NewHandler(mgr, nil, nil, nil)

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
			h := NewHandler(mgr, nil, nil, nil)

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
	h := NewHandler(mgr, sessions, nil, nil)

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
	h := NewHandler(mgr, nil, nil, nil)

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
		h := NewHandler(&mockUserManager{}, nil, nil, nil)
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
		h := NewHandler(mgr, nil, nil, nil)
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
		h := NewHandler(mgr, nil, nil, nil)
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
		h := NewHandler(&mockUserManager{}, nil, nil, nil)
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
		h := NewHandler(&mockUserManager{}, sessions, nil, nil)
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
		h := NewHandler(&mockUserManager{}, sessions, nil, nil)
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
		h := NewHandler(&mockUserManager{}, sessions, nil, nil)
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
		h := NewHandler(&mockUserManager{}, sessions, nil, nil)
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
		h := NewHandler(&mockUserManager{}, sessions, nil, nil)
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
		h := NewHandler(&mockUserManager{}, sessions, nil, nil)
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

// ── mocks for GetUserMfa ──────────────────────────────────────────────────────

type mockMfaRepo struct {
	enabled bool
	err     error
}

func (m *mockMfaRepo) IsEnabledForUser(_ context.Context, _ string) (bool, error) {
	return m.enabled, m.err
}

func (m *mockMfaRepo) EnabledForUsers(_ context.Context, userIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		out[id] = m.enabled
	}
	return out, m.err
}

type mockWebAuthnRepo struct {
	creds []webauthn.CredentialMeta
	err   error
}

func (m *mockWebAuthnRepo) ListMeta(_ context.Context, _ string) ([]webauthn.CredentialMeta, error) {
	return m.creds, m.err
}

func (m *mockWebAuthnRepo) CountByUsers(_ context.Context, userIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(userIDs))
	for _, id := range userIDs {
		out[id] = len(m.creds)
	}
	return out, m.err
}

func TestGetUserMfa(t *testing.T) {
	existingUser := &user.User{ID: "u1", Email: "u@example.com"}

	t.Run("user not found returns 404", func(t *testing.T) {
		mgr := &mockUserManager{findByIDErr: user.ErrNotFound}
		h := NewHandler(mgr, nil, nil, nil)
		req, _ := http.NewRequest("GET", "/", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.GetUserMfa(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d want=404", rr.Code)
		}
	})

	t.Run("user lookup DB error returns 500", func(t *testing.T) {
		mgr := &mockUserManager{findByIDErr: errors.New("db down")}
		h := NewHandler(mgr, nil, nil, nil)
		req, _ := http.NewRequest("GET", "/", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.GetUserMfa(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=500", rr.Code)
		}
	})

	t.Run("no mfa repos returns defaults", func(t *testing.T) {
		mgr := &mockUserManager{findByIDUser: existingUser}
		h := NewHandler(mgr, nil, nil, nil)
		req, _ := http.NewRequest("GET", "/", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.GetUserMfa(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=200", rr.Code)
		}
		var body map[string]any
		if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["totp_enabled"] != false {
			t.Fatalf("totp_enabled=%v want=false", body["totp_enabled"])
		}
	})

	t.Run("totp enabled propagates", func(t *testing.T) {
		mgr := &mockUserManager{findByIDUser: existingUser}
		mfa := &mockMfaRepo{enabled: true}
		h := NewHandler(mgr, nil, nil, mfa)
		req, _ := http.NewRequest("GET", "/", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.GetUserMfa(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=200", rr.Code)
		}
		var body map[string]any
		if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["totp_enabled"] != true {
			t.Fatalf("totp_enabled=%v want=true", body["totp_enabled"])
		}
	})

	t.Run("mfa DB error returns 500", func(t *testing.T) {
		mgr := &mockUserManager{findByIDUser: existingUser}
		mfa := &mockMfaRepo{err: errors.New("db error")}
		h := NewHandler(mgr, nil, nil, mfa)
		req, _ := http.NewRequest("GET", "/", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.GetUserMfa(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=500", rr.Code)
		}
	})

	t.Run("webauthn error returns 500", func(t *testing.T) {
		mgr := &mockUserManager{findByIDUser: existingUser}
		wa := &mockWebAuthnRepo{err: errors.New("db error")}
		h := NewHandler(mgr, nil, wa, nil)
		req, _ := http.NewRequest("GET", "/", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.GetUserMfa(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=500", rr.Code)
		}
	})
}

func TestBulkSetStatus(t *testing.T) {
	callerID := "caller-123"

	buildReq := func(body any, claimsID string) *http.Request {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest("POST", "/", bytes.NewReader(b))
		if claimsID != "" {
			req = withClaims(req, claimsID)
		}
		return req
	}

	t.Run("deactivates users and revokes sessions", func(t *testing.T) {
		mgr := &mockUserManager{}
		sess := &mockSessionManagerUnit{}
		h := NewHandler(mgr, sess, nil, nil)

		rr := httptest.NewRecorder()
		h.BulkSetStatus(rr, buildReq(map[string]any{
			"user_ids":  []string{"u1", "u2"},
			"is_active": false,
		}, callerID))

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=204", rr.Code)
		}
		if len(mgr.bulkSetActiveIDs) != 2 {
			t.Fatalf("want 2 IDs, got %v", mgr.bulkSetActiveIDs)
		}
		if mgr.bulkSetActiveValue != false {
			t.Fatal("expected is_active=false")
		}
		if sess.revokeAllCalls != 2 {
			t.Fatalf("revokeAllCalls=%d want=2", sess.revokeAllCalls)
		}
	})

	t.Run("activates users without revoking sessions", func(t *testing.T) {
		mgr := &mockUserManager{}
		sess := &mockSessionManagerUnit{}
		h := NewHandler(mgr, sess, nil, nil)

		rr := httptest.NewRecorder()
		h.BulkSetStatus(rr, buildReq(map[string]any{
			"user_ids":  []string{"u3"},
			"is_active": true,
		}, callerID))

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=204", rr.Code)
		}
		if sess.revokeAllCalls != 0 {
			t.Fatalf("revokeAllCalls=%d want=0 (no revoke on activate)", sess.revokeAllCalls)
		}
	})

	t.Run("excludes caller from set silently", func(t *testing.T) {
		mgr := &mockUserManager{}
		h := NewHandler(mgr, nil, nil, nil)

		rr := httptest.NewRecorder()
		h.BulkSetStatus(rr, buildReq(map[string]any{
			"user_ids":  []string{callerID, "u2"},
			"is_active": false,
		}, callerID))

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=204", rr.Code)
		}
		for _, id := range mgr.bulkSetActiveIDs {
			if id == callerID {
				t.Fatal("caller should have been excluded from bulk update")
			}
		}
	})

	t.Run("all IDs are caller → 204 with no repo call", func(t *testing.T) {
		mgr := &mockUserManager{}
		h := NewHandler(mgr, nil, nil, nil)

		rr := httptest.NewRecorder()
		h.BulkSetStatus(rr, buildReq(map[string]any{
			"user_ids":  []string{callerID},
			"is_active": false,
		}, callerID))

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=204", rr.Code)
		}
		if mgr.bulkSetActiveIDs != nil {
			t.Fatal("BulkSetActive should not have been called")
		}
	})

	t.Run("empty user_ids returns 400", func(t *testing.T) {
		h := NewHandler(&mockUserManager{}, nil, nil, nil)
		rr := httptest.NewRecorder()
		h.BulkSetStatus(rr, buildReq(map[string]any{"user_ids": []string{}, "is_active": false}, ""))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=400", rr.Code)
		}
	})

	t.Run("last_admin returns 409", func(t *testing.T) {
		mgr := &mockUserManager{bulkSetActiveErr: user.ErrLastAdmin}
		h := NewHandler(mgr, nil, nil, nil)
		rr := httptest.NewRecorder()
		h.BulkSetStatus(rr, buildReq(map[string]any{"user_ids": []string{"u1"}, "is_active": false}, callerID))
		if rr.Code != http.StatusConflict {
			t.Fatalf("status=%d want=409", rr.Code)
		}
	})

	t.Run("repo error returns 500", func(t *testing.T) {
		mgr := &mockUserManager{bulkSetActiveErr: errors.New("db error")}
		h := NewHandler(mgr, nil, nil, nil)
		rr := httptest.NewRecorder()
		h.BulkSetStatus(rr, buildReq(map[string]any{"user_ids": []string{"u1"}, "is_active": false}, callerID))
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=500", rr.Code)
		}
	})

	t.Run("invalid body returns 400", func(t *testing.T) {
		h := NewHandler(&mockUserManager{}, nil, nil, nil)
		req, _ := http.NewRequest("POST", "/", bytes.NewBufferString("not-json"))
		rr := httptest.NewRecorder()
		h.BulkSetStatus(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=400", rr.Code)
		}
	})
}

type mockLockoutManager struct {
	unlockEmail string
	unlockErr   error
}

func (m *mockLockoutManager) UnlockUser(ctx context.Context, email string) error {
	m.unlockEmail = email
	return m.unlockErr
}

func TestUnlockUser(t *testing.T) {
	t.Run("lockout manager unavailable", func(t *testing.T) {
		h := NewHandler(&mockUserManager{}, nil, nil, nil)
		req, _ := http.NewRequest("POST", "/api/v1/admin/users/u1/unlock", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.UnlockUser(rr, req)
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("user not found", func(t *testing.T) {
		mgr := &mockUserManager{findByIDErr: user.ErrNotFound}
		h := NewHandler(mgr, nil, nil, nil)
		h.SetLockoutManager(&mockLockoutManager{})
		req, _ := http.NewRequest("POST", "/api/v1/admin/users/u1/unlock", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.UnlockUser(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusNotFound)
		}
	})

	t.Run("unlock success", func(t *testing.T) {
		mgr := &mockUserManager{findByIDUser: &user.User{ID: "u1", Email: "test@example.com"}}
		h := NewHandler(mgr, nil, nil, nil)
		lm := &mockLockoutManager{}
		h.SetLockoutManager(lm)
		req, _ := http.NewRequest("POST", "/api/v1/admin/users/u1/unlock", nil)
		req = withURLParam(req, "id", "u1")
		rr := httptest.NewRecorder()
		h.UnlockUser(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusNoContent)
		}
		if lm.unlockEmail != "test@example.com" {
			t.Fatalf("expected email test@example.com, got %s", lm.unlockEmail)
		}
	})
}


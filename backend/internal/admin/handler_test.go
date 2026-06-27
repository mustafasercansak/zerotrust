package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/internal/session"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
)

type mockSessionManager struct {
	revokeAllCalled bool
	revokeIDCalled  bool
	listErr         bool
	revokeAllErr    bool
	revokeIDErr     bool
}

func (m *mockSessionManager) ListForUser(ctx context.Context, userID, currentHash string) ([]session.SessionInfo, error) {
	if m.listErr {
		return nil, errors.New("mock list error")
	}
	return []session.SessionInfo{{ID: "session1"}}, nil
}

func (m *mockSessionManager) RevokeAllForUser(ctx context.Context, userID string) error {
	if m.revokeAllErr {
		return errors.New("mock revoke all error")
	}
	m.revokeAllCalled = true
	return nil
}

func (m *mockSessionManager) RevokeByID(ctx context.Context, sessionID, userID string) error {
	if m.revokeIDErr {
		return errors.New("mock revoke id error")
	}
	m.revokeIDCalled = true
	return nil
}

func mockHandlerDeps(t *testing.T) (*Handler, *user.Service, *pgxpool.Pool, context.Context, *mockSessionManager) {
	t.Helper()
	dbURL := testdb.URL(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test db unreachable: %v", err)
	}
	if err := database.RunMigrations(dbURL, "../../migrations"); err != nil {
		pool.Close()
		t.Fatalf("migrations failed: %v", err)
	}
	repo := user.NewRepository(pool)
	svc := user.NewService(repo)

	if _, err := pool.Exec(ctx, "TRUNCATE TABLE users CASCADE"); err != nil {
		pool.Close()
		t.Fatalf("cleanup users failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO roles (name, description) VALUES ('viewer', 'Viewer')
		ON CONFLICT (name) DO NOTHING
	`); err != nil {
		pool.Close()
		t.Fatalf("seed roles failed: %v", err)
	}

	mockSessions := &mockSessionManager{}
	h := NewHandler(svc, mockSessions, nil, nil)

	return h, svc, pool, ctx, mockSessions
}

func TestHandler_ListUsers(t *testing.T) {
	h, svc, pool, ctx, _ := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	// Add users
	svc.Register(ctx, "u1@example.com", "Password1!", "en")
	svc.Register(ctx, "u2@example.com", "Password1!", "en")

	req, _ := http.NewRequest("GET", "/api/v1/admin/users?limit=10", nil)
	rr := httptest.NewRecorder()
	h.ListUsers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rr.Code)
	}

	var res pagedResponse[userResponse]
	json.NewDecoder(rr.Body).Decode(&res)
	if res.Total != 2 {
		t.Fatalf("Expected total=2, got %d", res.Total)
	}
}

func TestHandler_CreateUser(t *testing.T) {
	h, _, pool, _, _ := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	body := `{"email": "new@example.com", "first_name": "Alice", "last_name": "Smith", "password": "Password1!", "roles": ["admin"]}`
	req, _ := http.NewRequest("POST", "/api/v1/admin/users", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.CreateUser(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Expected 201 Created, got %d", rr.Code)
	}

	// Test conflict
	rrConflict := httptest.NewRecorder()
	reqConflict, _ := http.NewRequest("POST", "/api/v1/admin/users", bytes.NewBufferString(body))
	h.CreateUser(rrConflict, reqConflict)
	if rrConflict.Code != http.StatusConflict {
		t.Fatalf("Expected 409 Conflict, got %d", rrConflict.Code)
	}

	// Test bad json
	reqBad, _ := http.NewRequest("POST", "/api/v1/admin/users", bytes.NewBufferString("{bad"))
	rrBad := httptest.NewRecorder()
	h.CreateUser(rrBad, reqBad)
	if rrBad.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request, got %d", rrBad.Code)
	}

	// Test invalid email
	bodyBadEmail := `{"email": "notanemail", "password": "Password1!"}`
	reqBadEmail, _ := http.NewRequest("POST", "/api/v1/admin/users", bytes.NewBufferString(bodyBadEmail))
	rrBadEmail := httptest.NewRecorder()
	h.CreateUser(rrBadEmail, reqBadEmail)
	if rrBadEmail.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request, got %d", rrBadEmail.Code)
	}
}

func TestHandler_UpdateRoles(t *testing.T) {
	h, svc, pool, ctx, mockSessions := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	u, _ := svc.Register(ctx, "roles@example.com", "Password1!", "en")

	body := `{"roles": ["admin", "viewer"]}`
	req, _ := http.NewRequest("PATCH", "/api/v1/admin/users/"+u.ID+"/roles", bytes.NewBufferString(body))
	// Add chi URL param
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", u.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.UpdateRoles(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("Expected 204 No Content, got %d", rr.Code)
	}

	if !mockSessions.revokeAllCalled {
		t.Fatal("Expected revokeAllCalled to be true when updating roles")
	}

	// Test unknown role
	bodyBad := `{"roles": ["unknown"]}`
	reqBad, _ := http.NewRequest("PATCH", "/api/v1/admin/users/"+u.ID+"/roles", bytes.NewBufferString(bodyBad))
	reqBad = reqBad.WithContext(context.WithValue(reqBad.Context(), chi.RouteCtxKey, rctx))
	rrBad := httptest.NewRecorder()
	h.UpdateRoles(rrBad, reqBad)

	if rrBad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("Expected 422 Unprocessable Entity, got %d", rrBad.Code)
	}

	mockSessions.revokeAllErr = true
	bodyRevokeFail := `{"roles": ["viewer"]}`
	reqRevokeFail, _ := http.NewRequest("PATCH", "/api/v1/admin/users/"+u.ID+"/roles", bytes.NewBufferString(bodyRevokeFail))
	reqRevokeFail = reqRevokeFail.WithContext(context.WithValue(reqRevokeFail.Context(), chi.RouteCtxKey, rctx))
	rrRevokeFail := httptest.NewRecorder()
	h.UpdateRoles(rrRevokeFail, reqRevokeFail)
	if rrRevokeFail.Code != http.StatusInternalServerError {
		t.Fatalf("Expected 500 when role update cannot revoke sessions, got %d", rrRevokeFail.Code)
	}
}

func TestHandler_SetStatus(t *testing.T) {
	h, svc, pool, ctx, mockSessions := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	u, _ := svc.Register(ctx, "status@example.com", "Password1!", "en")

	body := `{"is_active": false}`
	req, _ := http.NewRequest("PATCH", "/api/v1/admin/users/"+u.ID+"/status", bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", u.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	h.SetStatus(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("Expected 204 No Content, got %d", rr.Code)
	}
	if !mockSessions.revokeAllCalled {
		t.Fatal("Expected revokeAllCalled to be true when deactivating user")
	}

	// Test Not Found
	bodyNotFound := `{"is_active": false}`
	reqNotFound, _ := http.NewRequest("PATCH", "/api/v1/admin/users/00000000-0000-0000-0000-000000000000/status", bytes.NewBufferString(bodyNotFound))
	rctxNotFound := chi.NewRouteContext()
	rctxNotFound.URLParams.Add("id", "00000000-0000-0000-0000-000000000000")
	reqNotFound = reqNotFound.WithContext(context.WithValue(reqNotFound.Context(), chi.RouteCtxKey, rctxNotFound))
	rrNotFound := httptest.NewRecorder()
	h.SetStatus(rrNotFound, reqNotFound)

	if rrNotFound.Code != http.StatusNotFound {
		t.Fatalf("Expected 404 Not Found, got %d", rrNotFound.Code)
	}
}

func TestHandler_SessionEndpoints(t *testing.T) {
	h, _, pool, _, mockSessions := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	// Test ListUserSessions
	reqList, _ := http.NewRequest("GET", "/api/v1/admin/users/123/sessions", nil)
	rrList := httptest.NewRecorder()
	h.ListUserSessions(rrList, reqList)
	if rrList.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %d", rrList.Code)
	}

	// Test RevokeAllUserSessions
	reqRevokeAll, _ := http.NewRequest("DELETE", "/api/v1/admin/users/123/sessions", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "123")
	reqRevokeAll = reqRevokeAll.WithContext(context.WithValue(reqRevokeAll.Context(), chi.RouteCtxKey, rctx))
	rrRevokeAll := httptest.NewRecorder()
	h.RevokeAllUserSessions(rrRevokeAll, reqRevokeAll)
	if rrRevokeAll.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d", rrRevokeAll.Code)
	}
	if !mockSessions.revokeAllCalled {
		t.Fatal("Expected revokeAllCalled to be true")
	}

	// Test RevokeUserSession
	reqRevokeOne, _ := http.NewRequest("DELETE", "/api/v1/admin/users/123/sessions/abc", nil)
	rctxOne := chi.NewRouteContext()
	rctxOne.URLParams.Add("id", "123")
	rctxOne.URLParams.Add("sessionId", "abc")
	reqRevokeOne = reqRevokeOne.WithContext(context.WithValue(reqRevokeOne.Context(), chi.RouteCtxKey, rctxOne))
	rrRevokeOne := httptest.NewRecorder()
	h.RevokeUserSession(rrRevokeOne, reqRevokeOne)
	if rrRevokeOne.Code != http.StatusNoContent {
		t.Fatalf("Expected 204, got %d", rrRevokeOne.Code)
	}
	if !mockSessions.revokeIDCalled {
		t.Fatal("Expected revokeIDCalled to be true")
	}
}

func TestHandler_NoSessionsManager(t *testing.T) {
	h, _, pool, _, _ := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	h.sessions = nil // Set sessions manager to nil to test error cases

	reqList, _ := http.NewRequest("GET", "/api/v1/admin/users/123/sessions", nil)
	rrList := httptest.NewRecorder()
	h.ListUserSessions(rrList, reqList)
	if rrList.Code != http.StatusServiceUnavailable {
		t.Fatalf("Expected 503, got %d", rrList.Code)
	}

	reqRevokeAll, _ := http.NewRequest("DELETE", "/api/v1/admin/users/123/sessions", nil)
	rrRevokeAll := httptest.NewRecorder()
	h.RevokeAllUserSessions(rrRevokeAll, reqRevokeAll)
	if rrRevokeAll.Code != http.StatusServiceUnavailable {
		t.Fatalf("Expected 503, got %d", rrRevokeAll.Code)
	}

	reqRevokeOne, _ := http.NewRequest("DELETE", "/api/v1/admin/users/123/sessions/abc", nil)
	rrRevokeOne := httptest.NewRecorder()
	h.RevokeUserSession(rrRevokeOne, reqRevokeOne)
	if rrRevokeOne.Code != http.StatusServiceUnavailable {
		t.Fatalf("Expected 503, got %d", rrRevokeOne.Code)
	}
}

func TestHandler_CoverageEdges(t *testing.T) {
	h, svc, pool, ctx, mockSessions := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	u, _ := svc.Register(ctx, "edge@example.com", "Password1!", "en")

	// UpdateRoles bad json
	reqUpdateBad, _ := http.NewRequest("PATCH", "/api/v1/admin/users/"+u.ID+"/roles", bytes.NewBufferString("{bad"))
	rrUpdateBad := httptest.NewRecorder()
	h.UpdateRoles(rrUpdateBad, reqUpdateBad)
	if rrUpdateBad.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d", rrUpdateBad.Code)
	}

	// SetStatus bad json
	reqStatusBad, _ := http.NewRequest("PATCH", "/api/v1/admin/users/"+u.ID+"/status", bytes.NewBufferString("{bad"))
	rrStatusBad := httptest.NewRecorder()
	h.SetStatus(rrStatusBad, reqStatusBad)
	if rrStatusBad.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d", rrStatusBad.Code)
	}

	// CreateUser invalid profile (too long name)
	bodyProfile := `{"email": "prof@example.com", "password": "Password1!", "first_name": "` + strings.Repeat("a", 85) + `"}`
	reqProfile, _ := http.NewRequest("POST", "/api/v1/admin/users", bytes.NewBufferString(bodyProfile))
	rrProfile := httptest.NewRecorder()
	h.CreateUser(rrProfile, reqProfile)
	if rrProfile.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400, got %d", rrProfile.Code)
	}

	// ListUserSessions internal error -> mockSessions.ListForUser error?
	mockSessions.listErr = true
	reqList, _ := http.NewRequest("GET", "/api/v1/admin/users/"+u.ID+"/sessions", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", u.ID)
	reqList = reqList.WithContext(context.WithValue(reqList.Context(), chi.RouteCtxKey, rctx))
	rrList := httptest.NewRecorder()
	h.ListUserSessions(rrList, reqList)
	if rrList.Code != http.StatusInternalServerError {
		t.Fatalf("Expected 500, got %d", rrList.Code)
	}

	// RevokeAll internal error
	mockSessions.revokeAllErr = true
	reqRevokeAll, _ := http.NewRequest("DELETE", "/api/v1/admin/users/"+u.ID+"/sessions", nil)
	reqRevokeAll = reqRevokeAll.WithContext(context.WithValue(reqRevokeAll.Context(), chi.RouteCtxKey, rctx))
	rrRevokeAll := httptest.NewRecorder()
	h.RevokeAllUserSessions(rrRevokeAll, reqRevokeAll)
	if rrRevokeAll.Code != http.StatusInternalServerError {
		t.Fatalf("Expected 500, got %d", rrRevokeAll.Code)
	}

	// RevokeOne internal error
	mockSessions.revokeIDErr = true
	reqRevokeOne, _ := http.NewRequest("DELETE", "/api/v1/admin/users/"+u.ID+"/sessions/abc", nil)
	rctxOne := chi.NewRouteContext()
	rctxOne.URLParams.Add("id", u.ID)
	rctxOne.URLParams.Add("sessionId", "abc")
	reqRevokeOne = reqRevokeOne.WithContext(context.WithValue(reqRevokeOne.Context(), chi.RouteCtxKey, rctxOne))
	rrRevokeOne := httptest.NewRecorder()
	h.RevokeUserSession(rrRevokeOne, reqRevokeOne)
	if rrRevokeOne.Code != http.StatusInternalServerError {
		t.Fatalf("Expected 500, got %d", rrRevokeOne.Code)
	}
}

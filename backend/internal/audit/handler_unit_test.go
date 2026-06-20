package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockAuditStore struct {
	listResult ListResult
	listErr    error
	lastParams ListParams
	trends     []TrendPoint
	trendsErr  error
	dashboard  SecurityDashboard
	dashErr    error
	lastRange  string
}

func (m *mockAuditStore) List(_ context.Context, p ListParams) (ListResult, error) {
	m.lastParams = p
	if m.listErr != nil {
		return ListResult{}, m.listErr
	}
	return m.listResult, nil
}

func (m *mockAuditStore) Trends(_ context.Context) ([]TrendPoint, error) {
	if m.trendsErr != nil {
		return nil, m.trendsErr
	}
	return m.trends, nil
}

func (m *mockAuditStore) SecurityDashboard(_ context.Context, rangeValue string) (SecurityDashboard, error) {
	m.lastRange = rangeValue
	if m.dashErr != nil {
		return SecurityDashboard{}, m.dashErr
	}
	return m.dashboard, nil
}

func TestHandlerListWithMockStore(t *testing.T) {
	store := &mockAuditStore{
		listResult: ListResult{
			Entries: []EntryRow{{ID: "1", Action: "login", Resource: "auth"}},
			Total:   1,
		},
	}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit?limit=10&offset=1&sort_by=action&sort_dir=asc&action=login&user_id=u1&resource=auth&outcome=success", nil)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
	}

	if store.lastParams.Limit != 10 || store.lastParams.Offset != 1 {
		t.Fatalf("unexpected paging params: %+v", store.lastParams)
	}
	if store.lastParams.SortBy != "action" || store.lastParams.SortDir != "asc" {
		t.Fatalf("unexpected sort params: %+v", store.lastParams)
	}
	if store.lastParams.Action != "login" || store.lastParams.UserID != "u1" || store.lastParams.Resource != "auth" || store.lastParams.Outcome != "success" {
		t.Fatalf("unexpected filter params: %+v", store.lastParams)
	}

	var payload pagedResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Total != 1 || len(payload.Data) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestHandlerListWithMockStoreError(t *testing.T) {
	store := &mockAuditStore{listErr: errors.New("boom")}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit", nil)
	rr := httptest.NewRecorder()

	h.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandlerTrendsWithMockStore(t *testing.T) {
	store := &mockAuditStore{trends: []TrendPoint{{Date: "2026-05-31", Success: 2, Failure: 1}}}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit/trends", nil)
	rr := httptest.NewRecorder()

	h.Trends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
	}

	var payload []TrendPoint
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 || payload[0].Success != 2 || payload[0].Failure != 1 {
		t.Fatalf("unexpected trends payload: %+v", payload)
	}
}

func TestHandlerTrendsWithMockStoreError(t *testing.T) {
	store := &mockAuditStore{trendsErr: errors.New("boom")}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit/trends", nil)
	rr := httptest.NewRecorder()

	h.Trends(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandlerSecurityDashboardWithMockStore(t *testing.T) {
	store := &mockAuditStore{
		dashboard: SecurityDashboard{
			Range:   "30d",
			Metrics: SecurityDashboardMetrics{FailedLogins: 4, ActiveSessions: 2},
		},
	}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/security-dashboard?range=30d", nil)
	rr := httptest.NewRecorder()

	h.SecurityDashboard(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
	}
	if store.lastRange != "30d" {
		t.Fatalf("range=%q want=30d", store.lastRange)
	}

	var payload SecurityDashboard
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Metrics.FailedLogins != 4 || payload.Metrics.ActiveSessions != 2 {
		t.Fatalf("unexpected dashboard payload: %+v", payload)
	}
}

func TestHandlerSecurityDashboardWithMockStoreError(t *testing.T) {
	store := &mockAuditStore{dashErr: errors.New("boom")}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/security-dashboard", nil)
	rr := httptest.NewRecorder()

	h.SecurityDashboard(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusInternalServerError)
	}
}

func TestHandlerExportCSV(t *testing.T) {
	email := "alice@example.com"
	uid := "u1"
	ip := "1.2.3.4"
	store := &mockAuditStore{
		listResult: ListResult{
			Entries: []EntryRow{
				{
					ID:        "e1",
					Action:    "auth.login",
					Resource:  "/api/v1/auth/login",
					UserEmail: &email,
					UserID:    &uid,
					IPAddress: &ip,
					CreatedAt: "2026-06-20T10:00:00Z",
				},
				{
					ID:        "e2",
					Action:    "users.create",
					Resource:  "/api/v1/admin/users",
					CreatedAt: "2026-06-20T09:00:00Z",
				},
			},
			Total: 2,
		},
	}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit/export?format=csv", nil)
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type=%q want text/csv; charset=utf-8", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); cd == "" {
		t.Fatal("missing Content-Disposition header")
	}

	body := rr.Body.String()
	// UTF-8 BOM
	if body[:3] != "\xef\xbb\xbf" {
		t.Fatal("missing UTF-8 BOM")
	}
	if !strings.Contains(body, "time,action,resource,user_email,user_id,ip_address") {
		t.Fatal("missing CSV header row")
	}
	if !strings.Contains(body, "auth.login") || !strings.Contains(body, "alice@example.com") {
		t.Fatal("missing first entry data")
	}
	if !strings.Contains(body, "users.create") {
		t.Fatal("missing second entry data")
	}

	// limit must be 10000, offset 0
	if store.lastParams.Limit != 10000 || store.lastParams.Offset != 0 {
		t.Fatalf("unexpected params: limit=%d offset=%d", store.lastParams.Limit, store.lastParams.Offset)
	}
}

func TestHandlerExportJSON(t *testing.T) {
	store := &mockAuditStore{
		listResult: ListResult{
			Entries: []EntryRow{
				{ID: "e1", Action: "auth.login", Resource: "/api/v1/auth/login", CreatedAt: "2026-06-20T10:00:00Z"},
			},
			Total: 1,
		},
	}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit/export?format=json", nil)
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type=%q want application/json", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); cd == "" {
		t.Fatal("missing Content-Disposition header")
	}

	var rows []EntryRow
	if err := json.NewDecoder(rr.Body).Decode(&rows); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(rows) != 1 || rows[0].Action != "auth.login" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestHandlerExportFiltersPassedToRepo(t *testing.T) {
	store := &mockAuditStore{listResult: ListResult{Entries: []EntryRow{}, Total: 0}}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit/export?format=csv&action=auth.login&user_id=u1&outcome=failure", nil)
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if store.lastParams.Action != "auth.login" {
		t.Fatalf("action=%q want auth.login", store.lastParams.Action)
	}
	if store.lastParams.UserID != "u1" {
		t.Fatalf("user_id=%q want u1", store.lastParams.UserID)
	}
	if store.lastParams.Outcome != "failure" {
		t.Fatalf("outcome=%q want failure", store.lastParams.Outcome)
	}
}

func TestHandlerExportDefaultsToCSV(t *testing.T) {
	store := &mockAuditStore{listResult: ListResult{Entries: []EntryRow{}, Total: 0}}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit/export", nil)
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type=%q want text/csv", ct)
	}
}

func TestHandlerExportStoreError(t *testing.T) {
	store := &mockAuditStore{listErr: errors.New("db down")}
	h := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/admin/audit/export?format=csv", nil)
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=500", rr.Code)
	}
}


func TestRepositorySetters(t *testing.T) {
	r := &Repository{}
	r.SetSecretsClient(nil)
	if r.secClient != nil {
		t.Fatal("expected secClient to be nil")
	}
	r.SetIPLocator(func(ip string) (string, string) { return "US", "New York" })
	if r.locator == nil {
		t.Fatal("expected locator to be set")
	}
	country, city := r.locator("any")
	if country != "US" || city != "New York" {
		t.Fatalf("locator returned %q %q want US New York", country, city)
	}
}

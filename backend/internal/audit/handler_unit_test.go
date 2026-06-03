package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockAuditStore struct {
	listResult ListResult
	listErr    error
	lastParams ListParams
	trends     []TrendPoint
	trendsErr  error
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

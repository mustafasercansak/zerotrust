package serviceaccount

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestToResponseNormalizesNilFields(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	resp := toResponse(&ServiceAccount{
		ID:        "sa1",
		Name:      "bot",
		ClientID:  "svc_1",
		IsActive:  true,
		CreatedAt: createdAt,
		Scopes:    nil,
	})

	if len(resp.Scopes) != 0 {
		t.Fatalf("scopes=%v want empty slice", resp.Scopes)
	}
	if resp.CreatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("created_at=%q want 2026-01-02T03:04:05Z", resp.CreatedAt)
	}
	if resp.ExpiresAt != nil {
		t.Fatalf("expires_at=%v want nil", resp.ExpiresAt)
	}
}

func TestToResponseFormatsExpiresAt(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	expiresAt := time.Date(2026, 1, 9, 23, 59, 59, 0, time.UTC)
	resp := toResponse(&ServiceAccount{
		ID:        "sa1",
		Name:      "bot",
		ClientID:  "svc_1",
		IsActive:  true,
		CreatedAt: createdAt,
		ExpiresAt: &expiresAt,
		Scopes:    []string{"users:read"},
	})

	if resp.ExpiresAt == nil || *resp.ExpiresAt != "2026-01-09T23:59:59Z" {
		t.Fatalf("expires_at=%v want 2026-01-09T23:59:59Z", resp.ExpiresAt)
	}
}

func TestWriteErrorAndQueryInt(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusForbidden, "forbidden")

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusForbidden)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type=%q want application/json", got)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "forbidden" {
		t.Fatalf("body=%v want forbidden error", body)
	}

	if got := queryInt("7", 25); got != 7 {
		t.Fatalf("queryInt valid=%d want=7", got)
	}
	if got := queryInt("-1", 25); got != 25 {
		t.Fatalf("queryInt negative=%d want=25", got)
	}
	if got := queryInt("bad", 25); got != 25 {
		t.Fatalf("queryInt invalid=%d want=25", got)
	}
}

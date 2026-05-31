package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/internal/user"
)

func mockHandlerDeps(t *testing.T) (*Handler, *Repository, *user.Repository, *pgxpool.Pool, context.Context) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		return nil, nil, nil, nil, nil
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test db unreachable: %v", err)
	}
	repo := NewRepository(pool)
	userRepo := user.NewRepository(pool)

	pool.Exec(ctx, "DELETE FROM audit_logs")
	pool.Exec(ctx, "DELETE FROM user_roles")
	pool.Exec(ctx, "DELETE FROM users")
	pool.Exec(ctx, "DELETE FROM roles")

	h := NewHandler(repo)

	return h, repo, userRepo, pool, ctx
}

func TestHandler_List(t *testing.T) {
	h, repo, userRepo, pool, ctx := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	// Seed data
	u, _ := userRepo.Create(ctx, "list@example.com", "hash", "en")

	err := repo.Log(ctx, Entry{Action: "test_action", Resource: "resource1", UserID: &u.ID, IPAddress: "127.0.0.1", UserAgent: "ua", Metadata: map[string]interface{}{"key": "val", "outcome": "success"}})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	req, _ := http.NewRequest("GET", "/api/v1/admin/audit?limit=10&action=test_action&outcome=success", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %v", rr.Code)
	}

	var res pagedResponse
	json.NewDecoder(rr.Body).Decode(&res)
	if res.Total != 1 {
		t.Fatalf("Expected 1 result, got %v", res.Total)
	}
	if len(res.Data) != 1 || res.Data[0].Action != "test_action" {
		t.Fatal("Data mismatch")
	}

	// Test Internal Error (simulate by closing pool)
	pool.Close()
	reqErr, _ := http.NewRequest("GET", "/api/v1/admin/audit", nil)
	rrErr := httptest.NewRecorder()
	h.List(rrErr, reqErr)
	if rrErr.Code != http.StatusInternalServerError {
		t.Fatalf("Expected 500 on closed DB, got %v", rrErr.Code)
	}
}

func TestHandler_Trends(t *testing.T) {
	h, repo, userRepo, pool, ctx := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	// Seed data for trends
	u1, _ := userRepo.Create(ctx, "trend1@example.com", "hash", "en")
	u2, _ := userRepo.Create(ctx, "trend2@example.com", "hash", "en")

	repo.Log(ctx, Entry{Action: "login", Resource: "auth", UserID: &u1.ID, IPAddress: "127.0.0.1", UserAgent: "ua", Metadata: map[string]interface{}{"outcome": "success"}})
	repo.Log(ctx, Entry{Action: "login", Resource: "auth", UserID: &u2.ID, IPAddress: "127.0.0.1", UserAgent: "ua", Metadata: map[string]interface{}{"outcome": "failure"}})

	req, _ := http.NewRequest("GET", "/api/v1/admin/audit/trends", nil)
	rr := httptest.NewRecorder()
	h.Trends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Expected 200, got %v", rr.Code)
	}

	var points []TrendPoint
	json.NewDecoder(rr.Body).Decode(&points)

	// There should be at least one day
	if len(points) == 0 {
		t.Fatal("Expected trend points")
	}

	// Find today's point
	today := time.Now().UTC().Format("2006-01-02")
	found := false
	for _, p := range points {
		if p.Date == today {
			found = true
			if p.Success != 1 || p.Failure != 1 {
				t.Fatalf("Expected 1 success, 1 failure for today, got %d/%d", p.Success, p.Failure)
			}
			break
		}
	}
	if !found {
		t.Fatal("Today's point not found")
	}

	// Test Internal Error
	pool.Close()
	reqErr, _ := http.NewRequest("GET", "/api/v1/admin/audit/trends", nil)
	rrErr := httptest.NewRecorder()
	h.Trends(rrErr, reqErr)
	if rrErr.Code != http.StatusInternalServerError {
		t.Fatalf("Expected 500 on closed DB, got %v", rrErr.Code)
	}
}

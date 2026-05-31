package audit

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
)

func setupTestDB(t *testing.T) (*pgxpool.Pool, context.Context, *Repository, *user.Repository) {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("failed to connect to test db: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test database not available: %v", err)
	}
	if err := database.RunMigrations(dsn, "../../migrations"); err != nil {
		pool.Close()
		t.Fatalf("migrations failed: %v", err)
	}

	if _, err := pool.Exec(ctx, "TRUNCATE TABLE audit_logs, users CASCADE"); err != nil {
		pool.Close()
		t.Fatalf("cleanup failed: %v", err)
	}

	return pool, ctx, NewRepository(pool), user.NewRepository(pool)
}

func seedAuditUser(t *testing.T, ctx context.Context, userRepo *user.Repository, email string) string {
	t.Helper()
	u, err := userRepo.Create(ctx, email, "hash", "en")
	if err != nil {
		t.Fatalf("seed user failed: %v", err)
	}
	return u.ID
}

func TestRepository_Log(t *testing.T) {
	pool, ctx, repo, userRepo := setupTestDB(t)
	defer pool.Close()
	uid := seedAuditUser(t, ctx, userRepo, "audit-log@example.com")

	err := repo.Log(ctx, Entry{
		UserID:    &uid,
		Action:    "test.action",
		Resource:  "/test",
		IPAddress: "127.0.0.1",
		UserAgent: "TestAgent/1.0",
		Metadata:  map[string]any{"key": "value"},
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	err = repo.Log(ctx, Entry{Action: "no.metadata", Resource: "test"})
	if err != nil {
		t.Fatalf("Log without metadata failed: %v", err)
	}

	err = repo.Log(ctx, Entry{Action: "bad.metadata", Metadata: map[string]any{"bad": make(chan int)}})
	if err == nil {
		t.Fatal("expected marshal error for unsupported metadata type")
	}
}

func TestRepository_ListAndTrends(t *testing.T) {
	pool, ctx, repo, userRepo := setupTestDB(t)
	defer pool.Close()
	uid := seedAuditUser(t, ctx, userRepo, "audit-list@example.com")

	if err := repo.Log(ctx, Entry{
		UserID:   &uid,
		Action:   "auth.login",
		Resource: "auth",
		Metadata: map[string]any{"outcome": "success"},
	}); err != nil {
		t.Fatalf("log success event failed: %v", err)
	}
	if err := repo.Log(ctx, Entry{
		UserID:   &uid,
		Action:   "auth.login",
		Resource: "auth",
		Metadata: map[string]any{"outcome": "failure"},
	}); err != nil {
		t.Fatalf("log failure event failed: %v", err)
	}

	res, err := repo.List(ctx, ListParams{
		Limit:   10,
		Offset:  0,
		SortBy:  "created_at",
		SortDir: "desc",
		Outcome: "success",
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if res.Total != 1 {
		t.Errorf("List Total = %d, want 1", res.Total)
	}
	if len(res.Entries) != 1 || res.Entries[0].Metadata["outcome"] != "success" {
		t.Errorf("List Entries mismatch = %+v", res.Entries)
	}

	resFallback, err := repo.List(ctx, ListParams{Limit: 999, SortBy: "bad_col"})
	if err != nil {
		t.Fatalf("List fallback failed: %v", err)
	}
	if resFallback.Total != 2 {
		t.Fatalf("List fallback total = %d, want 2", resFallback.Total)
	}

	trends, err := repo.Trends(ctx)
	if err != nil {
		t.Fatalf("Trends failed: %v", err)
	}
	if len(trends) != 7 {
		t.Errorf("Trends length = %d, want 7", len(trends))
	}

	todayFound := false
	for _, point := range trends {
		if point.Date == trends[len(trends)-1].Date {
			if point.Success == 1 && point.Failure == 1 {
				todayFound = true
			}
		}
	}
	if !todayFound {
		t.Errorf("expected today's trend point with 1 success and 1 failure: %+v", trends)
	}
}

func TestRepository_ListAndTrendsClosedDB(t *testing.T) {
	pool, ctx, repo, _ := setupTestDB(t)
	pool.Close()

	if _, err := repo.List(ctx, ListParams{}); err == nil {
		t.Fatal("expected list error after pool close")
	}
	if _, err := repo.Trends(ctx); err == nil {
		t.Fatal("expected trends error after pool close")
	}
}

func TestNullStrReturnsExpectedPointers(t *testing.T) {
	if got := nullStr(""); got != nil {
		t.Fatalf("nullStr empty got=%v want=nil", got)
	}

	v := "127.0.0.1"
	if got := nullStr(v); got == nil || *got != v {
		t.Fatalf("nullStr non-empty got=%v want=%s", got, v)
	}
}

func TestRepositoryLogClosedPoolWrapsInsertError(t *testing.T) {
	pool, ctx, repo, userRepo := setupTestDB(t)
	uid := seedAuditUser(t, ctx, userRepo, "audit-closed@example.com")
	pool.Close()

	err := repo.Log(ctx, Entry{UserID: &uid, Action: "closed.db"})
	if err == nil {
		t.Fatal("expected insert error after pool close")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected context cancellation error: %v", err)
	}
}

package audit

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	ctx := context.Background()
	dsn := "postgres://postgres:postgres@localhost:5432/zerotrust_test?sslmode=disable"
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("test database not available: %v", err)
	}

	return pool
}

func TestRepository_Log(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool)
	ctx := context.Background()

	uid := "test-user-id"
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
}

func TestRepository_ListAndTrends(t *testing.T) {
	pool := setupTestDB(t)
	defer pool.Close()

	repo := NewRepository(pool)
	ctx := context.Background()

	// Clear out any old rows
	_, _ = pool.Exec(ctx, "DELETE FROM audit_logs")

	uid := "user-123"
	// Insert
	repo.Log(ctx, Entry{
		UserID:   &uid,
		Action:   "auth.login",
		Metadata: map[string]any{"outcome": "success"},
	})
	repo.Log(ctx, Entry{
		UserID:   &uid,
		Action:   "auth.login",
		Metadata: map[string]any{"outcome": "failure"},
	})
	
	time.Sleep(10 * time.Millisecond)

	// List
	res, err := repo.List(ctx, ListParams{
		Limit:  10,
		Offset: 0,
		SortBy: "created_at",
		SortDir: "desc",
	})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if res.Total != 2 {
		t.Errorf("List Total = %d, want 2", res.Total)
	}
	if len(res.Entries) != 2 {
		t.Errorf("List Entries count = %d, want 2", len(res.Entries))
	}

	// Trends
	trends, err := repo.Trends(ctx)
	if err != nil {
		t.Fatalf("Trends failed: %v", err)
	}
	if len(trends) == 0 {
		t.Errorf("Trends returned empty results")
	}
}

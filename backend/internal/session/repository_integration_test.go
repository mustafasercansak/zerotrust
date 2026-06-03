package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/pkg/database"
)

func setupSessionIntegrationRepo(t *testing.T) (*Repository, string, *pgxpool.Pool, context.Context) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
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
	if err := database.RunMigrations(dbURL, "../../migrations"); err != nil {
		pool.Close()
		t.Fatalf("migrations failed: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE TABLE sessions, users, roles CASCADE"); err != nil {
		pool.Close()
		t.Fatalf("cleanup failed: %v", err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, locale) VALUES ('sess-repo@example.com', 'hash', 'en') RETURNING id::text
	`).Scan(&userID); err != nil {
		pool.Close()
		t.Fatalf("create user: %v", err)
	}
	return NewRepository(pool, NewEventHub()), userID, pool, ctx
}

func hashTok(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func TestSessionRepository_CreateAndListForUser(t *testing.T) {
	repo, userID, pool, ctx := setupSessionIntegrationRepo(t)
	defer pool.Close()

	expiry := time.Now().Add(time.Hour)
	h1, h2 := hashTok("tok-1"), hashTok("tok-2")

	if err := repo.Create(ctx, userID, h1, "1.2.3.4:1234", "Mozilla/5.0", nil, expiry); err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	if err := repo.Create(ctx, userID, h2, "5.6.7.8:5678", "curl/8.0", nil, expiry); err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	sessions, err := repo.ListForUser(ctx, userID, h1)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions=%d want=2", len(sessions))
	}
	if !sessions[0].IsCurrent {
		t.Fatal("first result should be current session")
	}
}

func TestSessionRepository_FindUserIDByHash(t *testing.T) {
	repo, userID, pool, ctx := setupSessionIntegrationRepo(t)
	defer pool.Close()

	h := hashTok("find-hash-token")
	if err := repo.Create(ctx, userID, h, "127.0.0.1", "agent", nil, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	gotID, err := repo.FindUserIDByHash(ctx, h)
	if err != nil {
		t.Fatalf("FindUserIDByHash: %v", err)
	}
	if gotID != userID {
		t.Fatalf("userID=%q want=%q", gotID, userID)
	}

	_, err = repo.FindUserIDByHash(ctx, hashTok("nonexistent"))
	if err != ErrNotFound {
		t.Fatalf("missing hash: want ErrNotFound, got %v", err)
	}
}

func TestSessionRepository_RevokeByID(t *testing.T) {
	repo, userID, pool, ctx := setupSessionIntegrationRepo(t)
	defer pool.Close()

	h := hashTok("revoke-by-id-token")
	if err := repo.Create(ctx, userID, h, "127.0.0.1", "agent", nil, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	sessions, _ := repo.ListForUser(ctx, userID, "")
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	id := sessions[0].ID

	if err := repo.RevokeByID(ctx, id, "00000000-0000-0000-0000-000000000000"); err != ErrNotFound {
		t.Fatalf("wrong user: want ErrNotFound, got %v", err)
	}
	if err := repo.RevokeByID(ctx, id, userID); err != nil {
		t.Fatalf("RevokeByID: %v", err)
	}
	if err := repo.RevokeByID(ctx, id, userID); err != ErrNotFound {
		t.Fatalf("duplicate revoke: want ErrNotFound, got %v", err)
	}
}

func TestSessionRepository_RevokeForDevice(t *testing.T) {
	repo, userID, pool, ctx := setupSessionIntegrationRepo(t)
	defer pool.Close()

	ip, ua, dev := "10.0.0.1", "TestAgent/1.0", map[string]string{"os": "linux"}
	if err := repo.Create(ctx, userID, hashTok("device-token"), ip, ua, dev, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.RevokeForDevice(ctx, userID, ip, ua, dev); err != nil {
		t.Fatalf("RevokeForDevice: %v", err)
	}

	sessions, _ := repo.ListForUser(ctx, userID, "")
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions after device revoke, got %d", len(sessions))
	}
}

func TestSessionRepository_EvictExcessSessions(t *testing.T) {
	repo, userID, pool, ctx := setupSessionIntegrationRepo(t)
	defer pool.Close()

	expiry := time.Now().Add(time.Hour)
	for i := 0; i < 3; i++ {
		if err := repo.Create(ctx, userID, hashTok(fmt.Sprintf("evict-%d", i)), "127.0.0.1", "agent", nil, expiry); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	if err := repo.EvictExcessSessions(ctx, userID, 1); err != nil {
		t.Fatalf("EvictExcessSessions: %v", err)
	}

	sessions, _ := repo.ListForUser(ctx, userID, "")
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after evict, got %d", len(sessions))
	}
}

func TestSessionRepository_DeleteExpiredAndRevokeStale(t *testing.T) {
	repo, userID, pool, ctx := setupSessionIntegrationRepo(t)
	defer pool.Close()

	if err := repo.Create(ctx, userID, hashTok("active-tok"), "127.0.0.1", "agent", nil, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("Create active: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, expires_at)
		VALUES ($1, $2, now() - interval '1 hour')
	`, userID, hashTok("expired-tok")); err != nil {
		t.Fatalf("insert expired: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, expires_at, created_at)
		VALUES ($1, $2, now() + interval '1 hour', now() - interval '5 minutes')
	`, userID, hashTok("stale-tok")); err != nil {
		t.Fatalf("insert stale: %v", err)
	}

	n, err := repo.RevokeStaleInitialSessions(ctx)
	if err != nil {
		t.Fatalf("RevokeStaleInitialSessions: %v", err)
	}
	if n != 1 {
		t.Fatalf("stale revoked=%d want=1", n)
	}

	deleted, err := repo.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if deleted < 2 {
		t.Fatalf("deleted=%d want>=2", deleted)
	}

	sessions, _ := repo.ListForUser(ctx, userID, "")
	if len(sessions) != 1 {
		t.Fatalf("expected 1 active session, got %d", len(sessions))
	}
}

func TestSessionRepository_GetActiveSessions(t *testing.T) {
	repo, userID, pool, ctx := setupSessionIntegrationRepo(t)
	defer pool.Close()

	for i := 0; i < 2; i++ {
		if err := repo.Create(ctx, userID, hashTok(fmt.Sprintf("get-active-%d", i)), "127.0.0.1", "agent", nil, time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	active, err := repo.GetActiveSessions(ctx, userID)
	if err != nil {
		t.Fatalf("GetActiveSessions: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("active=%d want=2", len(active))
	}
}

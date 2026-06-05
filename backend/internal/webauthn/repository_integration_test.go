package webauthn

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/pkg/database"
)

func setupRepo(t *testing.T) (*Repository, *pgxpool.Pool, context.Context, string) {
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
	if _, err := pool.Exec(ctx, "TRUNCATE TABLE users CASCADE"); err != nil {
		pool.Close()
		t.Fatalf("cleanup failed: %v", err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email, email_hash, password_hash) VALUES ('wa@example.com', 'hash', 'x') RETURNING id::text
	`).Scan(&userID); err != nil {
		pool.Close()
		t.Fatalf("seed user failed: %v", err)
	}
	return NewRepository(pool), pool, ctx, userID
}

func TestRepositoryWebAuthnLifecycle(t *testing.T) {
	repo, pool, ctx, userID := setupRepo(t)
	defer pool.Close()

	// Empty to start.
	if n, err := repo.Count(ctx, userID); err != nil || n != 0 {
		t.Fatalf("Count empty = %d, err=%v", n, err)
	}
	if exists, err := repo.CredentialExists(ctx, "cred-1"); err != nil || exists {
		t.Fatalf("CredentialExists empty = %v, err=%v", exists, err)
	}

	// Insert a credential.
	if err := repo.Insert(ctx, userID, "cred-1", []byte(`{"id":"AQ"}`), 0, "YubiKey"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if n, _ := repo.Count(ctx, userID); n != 1 {
		t.Fatalf("Count after insert = %d, want 1", n)
	}
	if exists, _ := repo.CredentialExists(ctx, "cred-1"); !exists {
		t.Fatal("CredentialExists should be true after insert")
	}

	// ListData and ListMeta. (jsonb normalizes formatting, so compare semantically.)
	blobs, err := repo.ListData(ctx, userID)
	if err != nil || len(blobs) != 1 {
		t.Fatalf("ListData = %v, err=%v", blobs, err)
	}
	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(blobs[0], &got); err != nil || got.ID != "AQ" {
		t.Fatalf("ListData blob = %s, parsed id=%q err=%v", blobs[0], got.ID, err)
	}
	metas, err := repo.ListMeta(ctx, userID)
	if err != nil || len(metas) != 1 || metas[0].Name != "YubiKey" || metas[0].LastUsedAt != nil {
		t.Fatalf("ListMeta = %+v, err=%v", metas, err)
	}
	credRowID := metas[0].ID

	// UpdateOnLogin bumps the counter and sets last_used_at.
	if err := repo.UpdateOnLogin(ctx, "cred-1", []byte(`{"id":"AQ","n":2}`), 5); err != nil {
		t.Fatalf("UpdateOnLogin: %v", err)
	}
	metas, _ = repo.ListMeta(ctx, userID)
	if metas[0].SignCount != 5 || metas[0].LastUsedAt == nil {
		t.Fatalf("after login meta = %+v", metas[0])
	}

	// Delete is scoped to the owner.
	if err := repo.Delete(ctx, credRowID, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete by wrong owner = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, credRowID, userID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if n, _ := repo.Count(ctx, userID); n != 0 {
		t.Fatalf("Count after delete = %d, want 0", n)
	}
}

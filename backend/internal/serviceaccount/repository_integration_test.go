package serviceaccount

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/pkg/database"
)

func setupSARepo(t *testing.T) (*Repository, *pgxpool.Pool, context.Context) {
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
	if _, err := pool.Exec(ctx, "TRUNCATE TABLE service_account_scopes, service_accounts, users CASCADE"); err != nil {
		pool.Close()
		t.Fatalf("cleanup failed: %v", err)
	}
	return NewRepository(pool), pool, ctx
}

func TestSARepository_CreateAndFind(t *testing.T) {
	repo, pool, ctx := setupSARepo(t)
	defer pool.Close()

	sa, secret, err := repo.Create(ctx, "test-sa", "", []string{"tokens:validate", "users:read"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sa.ClientID == "" || !sa.IsActive {
		t.Fatalf("unexpected service account: %+v", sa)
	}
	if len(sa.Scopes) != 2 {
		t.Fatalf("scopes=%d want=2", len(sa.Scopes))
	}
	if secret == "" {
		t.Fatal("expected non-empty secret")
	}

	found, err := repo.FindByClientID(ctx, sa.ClientID)
	if err != nil {
		t.Fatalf("FindByClientID: %v", err)
	}
	if found.ID != sa.ID {
		t.Fatalf("id mismatch: got=%q want=%q", found.ID, sa.ID)
	}

	_, err = repo.FindByClientID(ctx, "nonexistent")
	if err != ErrNotFound {
		t.Fatalf("missing clientID: want ErrNotFound, got %v", err)
	}
}

func TestSARepository_CreateDuplicateName(t *testing.T) {
	repo, pool, ctx := setupSARepo(t)
	defer pool.Close()

	if _, _, err := repo.Create(ctx, "dup-sa", "", nil, nil); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, _, err := repo.Create(ctx, "dup-sa", "", nil, nil)
	if err != ErrNameTaken {
		t.Fatalf("duplicate name: want ErrNameTaken, got %v", err)
	}
}

func TestSARepository_List(t *testing.T) {
	repo, pool, ctx := setupSARepo(t)
	defer pool.Close()

	for i := 0; i < 3; i++ {
		name := "list-sa-" + string(rune('a'+i))
		if _, _, err := repo.Create(ctx, name, "", nil, nil); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	result, err := repo.List(ctx, ListParams{Limit: 25})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Accounts) != 3 {
		t.Fatalf("accounts=%d want=3", len(result.Accounts))
	}
}

func TestSARepository_Update(t *testing.T) {
	repo, pool, ctx := setupSARepo(t)
	defer pool.Close()

	sa, _, err := repo.Create(ctx, "update-sa", "", []string{"users:read"}, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := repo.Update(ctx, sa.ID, "updated-sa", []string{"tokens:validate"}, nil, true)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "updated-sa" {
		t.Fatalf("name=%q want=updated-sa", updated.Name)
	}
	if len(updated.Scopes) != 1 || updated.Scopes[0] != "tokens:validate" {
		t.Fatalf("scopes=%v want=[tokens:validate]", updated.Scopes)
	}

	_, err = repo.Update(ctx, "00000000-0000-0000-0000-000000000000", "x", nil, nil, true)
	if err != ErrNotFound {
		t.Fatalf("missing id: want ErrNotFound, got %v", err)
	}
}

func TestSARepository_SetActive(t *testing.T) {
	repo, pool, ctx := setupSARepo(t)
	defer pool.Close()

	sa, _, err := repo.Create(ctx, "active-sa", "", nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.SetActive(ctx, sa.ID, false); err != nil {
		t.Fatalf("SetActive false: %v", err)
	}
	found, _ := repo.FindByClientID(ctx, sa.ClientID)
	if found.IsActive {
		t.Fatal("expected is_active=false after deactivation")
	}

	if err := repo.SetActive(ctx, sa.ID, true); err != nil {
		t.Fatalf("SetActive true: %v", err)
	}

	if err := repo.SetActive(ctx, "00000000-0000-0000-0000-000000000000", false); err != ErrNotFound {
		t.Fatalf("missing id: want ErrNotFound, got %v", err)
	}
}

func TestSARepository_Revoke(t *testing.T) {
	repo, pool, ctx := setupSARepo(t)
	defer pool.Close()

	sa, _, err := repo.Create(ctx, "revoke-sa", "", nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Revoke(ctx, sa.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if err := repo.Revoke(ctx, "00000000-0000-0000-0000-000000000000"); err != ErrNotFound {
		t.Fatalf("missing id: want ErrNotFound, got %v", err)
	}
}

func TestSARepository_CheckSecret(t *testing.T) {
	repo, pool, ctx := setupSARepo(t)
	defer pool.Close()

	sa, secret, err := repo.Create(ctx, "secret-sa", "", nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !repo.CheckSecret(sa.ClientSecretHash, secret) {
		t.Fatal("CheckSecret should return true for correct secret")
	}
	if repo.CheckSecret(sa.ClientSecretHash, "wrong-secret") {
		t.Fatal("CheckSecret should return false for wrong secret")
	}
}

func TestSARepository_AllPermissions(t *testing.T) {
	repo, pool, ctx := setupSARepo(t)
	defer pool.Close()

	perms, err := repo.allPermissions(ctx)
	if err != nil {
		t.Fatalf("allPermissions: %v", err)
	}
	if len(perms) == 0 {
		t.Fatal("expected at least some permissions seeded")
	}
	if !perms["users:read"] {
		t.Fatal("expected users:read permission to exist")
	}
}

func TestSARepository_RotateSecret(t *testing.T) {
	repo, pool, ctx := setupSARepo(t)
	defer pool.Close()

	sa, originalSecret, err := repo.Create(ctx, "rotate-sa", "", nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, newSecret, err := repo.RotateSecret(ctx, sa.ID)
	if err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}
	if newSecret == "" || newSecret == originalSecret {
		t.Fatal("expected new distinct secret after rotation")
	}
	if updated.OldClientSecretHash == nil {
		t.Fatal("expected old secret hash to be stored")
	}
	if updated.OldSecretExpiresAt == nil {
		t.Fatal("expected old secret expiry to be set")
	}

	if !repo.CheckSecret(updated.ClientSecretHash, newSecret) {
		t.Fatal("new secret should verify against new hash")
	}
	if !repo.CheckSecret(*updated.OldClientSecretHash, originalSecret) {
		t.Fatal("old secret should still verify during grace period")
	}
}

func TestSARepository_CreateWithExpiry(t *testing.T) {
	repo, pool, ctx := setupSARepo(t)
	defer pool.Close()

	exp := time.Now().Add(24 * time.Hour)
	sa, _, err := repo.Create(ctx, "expiry-sa", "", nil, &exp)
	if err != nil {
		t.Fatalf("Create with expiry: %v", err)
	}
	if sa.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
}

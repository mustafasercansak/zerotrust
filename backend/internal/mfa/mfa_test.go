package mfa

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
)

func mockHandlerDeps(t *testing.T) (*Handler, *Service, *Repository, *user.Repository, *pgxpool.Pool, context.Context) {
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
	repo := NewRepository(pool)
	userRepo := user.NewRepository(pool)
	key := []byte("thisis32byteslongsecretkey123456")
	svc := NewService(repo, key, nil)

	if _, err := pool.Exec(ctx, "DELETE FROM user_mfa"); err != nil {
		pool.Close()
		t.Fatalf("cleanup user_mfa failed: %v", err)
	}
	if _, err := pool.Exec(ctx, "DELETE FROM users"); err != nil {
		pool.Close()
		t.Fatalf("cleanup users failed: %v", err)
	}

	h := NewHandler(svc, nil, 0)
	return h, svc, repo, userRepo, pool, ctx
}

func TestMFAIntegration(t *testing.T) {
	h, svc, repo, userRepo, pool, ctx := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	u, err := userRepo.Create(ctx, "mfa@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("User create failed: %v", err)
	}

	// 1. Setup MFA
	qr, secret, codes, err := svc.Setup(ctx, u.ID, "mfa@example.com", "")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if qr == "" || secret == "" || len(codes) != 10 && len(codes) != 8 {
		t.Fatal("Invalid setup response")
	}

	// 2. Validate pending secret
	if svc.IsEnabled(ctx, u.ID) {
		t.Fatal("Should not be enabled")
	}

	// Try verifying with invalid code
	err = svc.VerifyAndEnable(ctx, u.ID, "000000")
	if err == nil {
		t.Fatal("Expected error with invalid code")
	}

	// Directly insert pending to simulate state for other tests if needed
	repo.UpsertPending(ctx, u.ID, "enc", []string{"code1"})

	// Get recovery codes
	rc, _ := repo.RecoveryCodes(ctx, u.ID)
	if len(rc) != 0 {
		t.Fatal("Should have no active recovery codes yet")
	}

	// Disable
	repo.Enable(ctx, u.ID)
	if !repo.IsEnabled(ctx, u.ID) {
		t.Fatal("Should be enabled")
	}

	err = svc.Disable(ctx, u.ID, "000000")
	if err == nil {
		t.Fatal("Disable should fail with bad code")
	}

	repo.Delete(ctx, u.ID)
}

type mockRepo struct{ store }

func (m mockRepo) IsEnabled(ctx context.Context, userID string) bool { return false }

// TestRecoveryCodeConcurrentUse verifies that concurrent validation of the
// same recovery code succeeds exactly once: consumption is atomic under a
// per-user row lock (#95).
func TestRecoveryCodeConcurrentUse(t *testing.T) {
	h, svc, repo, userRepo, pool, ctx := mockHandlerDeps(t)
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	u, err := userRepo.Create(ctx, "mfa-race@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("User create failed: %v", err)
	}

	_, _, rawCodes, err := svc.Setup(ctx, u.ID, "mfa-race@example.com", "")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	if err := repo.Enable(ctx, u.ID); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	const attempts = 16
	var wg sync.WaitGroup
	var successes int64
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if svc.Validate(ctx, u.ID, rawCodes[0]) {
				atomic.AddInt64(&successes, 1)
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("concurrent use of the same recovery code succeeded %d times, want exactly 1", successes)
	}
}

func TestMFAInvalidKey(t *testing.T) {
	_, _, _, err := NewService(mockRepo{}, []byte("short"), nil).Setup(context.Background(), "id", "email", "")
	if err == nil {
		t.Fatal("Expected error with short key")
	}
}

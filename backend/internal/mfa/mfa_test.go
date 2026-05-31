package mfa

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/internal/user"
)

func mockHandlerDeps() (*Handler, *Service, *Repository, *user.Repository, *pgxpool.Pool, context.Context) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		return nil, nil, nil, nil, nil, nil
	}
	ctx := context.Background()
	pool, _ := pgxpool.New(ctx, dbURL)
	repo := NewRepository(pool)
	userRepo := user.NewRepository(pool)
	key := []byte("thisis32byteslongsecretkey123456")
	svc := NewService(repo, key, nil)
	
	pool.Exec(ctx, "DELETE FROM mfa")
	pool.Exec(ctx, "DELETE FROM users")
	
	h := NewHandler(svc, nil, 0)
	return h, svc, repo, userRepo, pool, ctx
}

func TestMFAIntegration(t *testing.T) {
	h, svc, repo, userRepo, pool, ctx := mockHandlerDeps()
	if h == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	defer pool.Close()

	u, err := userRepo.Create(ctx, "mfa@example.com", "hash", "en")
	if err != nil { t.Fatalf("User create failed: %v", err) }
	
	// 1. Setup MFA
	qr, secret, codes, err := svc.Setup(ctx, u.ID, "mfa@example.com", "")
	if err != nil { t.Fatalf("Setup failed: %v", err) }
	if qr == "" || secret == "" || len(codes) != 10 && len(codes) != 8 { t.Fatal("Invalid setup response") }

	// 2. Validate pending secret
	if svc.IsEnabled(ctx, u.ID) { t.Fatal("Should not be enabled") }

	// Try verifying with invalid code
	err = svc.VerifyAndEnable(ctx, u.ID, "000000")
	if err == nil { t.Fatal("Expected error with invalid code") }

	// Directly insert pending to simulate state for other tests if needed
	repo.UpsertPending(ctx, u.ID, "enc", []string{"code1"})

	// Get recovery codes
	rc, _ := repo.RecoveryCodes(ctx, u.ID)
	if len(rc) != 0 {
		t.Fatal("Should have no active recovery codes yet")
	}

	// Disable
	repo.Enable(ctx, u.ID)
	if !repo.IsEnabled(ctx, u.ID) { t.Fatal("Should be enabled") }
	
	err = svc.Disable(ctx, u.ID, "000000")
	if err == nil { t.Fatal("Disable should fail with bad code") }
	
	repo.Delete(ctx, u.ID)
}

type mockRepo struct { store }
func (m mockRepo) IsEnabled(ctx context.Context, userID string) bool { return false }

func TestMFAInvalidKey(t *testing.T) {
	_, _, _, err := NewService(mockRepo{}, []byte("short"), nil).Setup(context.Background(), "id", "email", "")
	if err == nil { t.Fatal("Expected error with short key") }
}

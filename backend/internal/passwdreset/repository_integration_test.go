package passwdreset

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
	"golang.org/x/crypto/bcrypt"
)

func setupResetRepo(t *testing.T) (*Repository, *user.Repository, *pgxpool.Pool, context.Context) {
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

	if _, err := pool.Exec(ctx, "TRUNCATE TABLE password_reset_tokens, sessions, user_roles, users CASCADE"); err != nil {
		pool.Close()
		t.Fatalf("cleanup failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO roles (name, description) VALUES ('viewer', 'Viewer')
		ON CONFLICT (name) DO NOTHING
	`); err != nil {
		pool.Close()
		t.Fatalf("seed roles failed: %v", err)
	}

	return NewRepository(pool), user.NewRepository(pool), pool, ctx
}

func TestRepositoryCreateInvalidatesPreviousTokens(t *testing.T) {
	repo, userRepo, pool, ctx := setupResetRepo(t)
	defer pool.Close()

	u, err := userRepo.Create(ctx, "reset-create@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	first, err := repo.Create(ctx, u.ID)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	second, err := repo.Create(ctx, u.ID)
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}
	if first == second {
		t.Fatal("expected unique reset tokens")
	}

	var total, active int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE used_at IS NULL)
		FROM password_reset_tokens
		WHERE user_id = $1::uuid
	`, u.ID).Scan(&total, &active)
	if err != nil {
		t.Fatalf("count tokens failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("total tokens=%d want=2", total)
	}
	if active != 1 {
		t.Fatalf("active tokens=%d want=1", active)
	}

	err = repo.ConsumeAndReset(ctx, first, "new-pass")
	if !errors.Is(err, ErrUsed) {
		t.Fatalf("consume old token err=%v want=%v", err, ErrUsed)
	}
}

func TestRepositoryConsumeAndResetSuccessRevokesSessions(t *testing.T) {
	repo, userRepo, pool, ctx := setupResetRepo(t)
	defer pool.Close()

	u, err := userRepo.Create(ctx, "reset-consume@example.com", "old-hash", "en")
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, expires_at, is_revoked)
		VALUES ($1::uuid, 'session-hash', NOW() + INTERVAL '1 hour', false)
	`, u.ID); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	rawToken, err := repo.Create(ctx, u.ID)
	if err != nil {
		t.Fatalf("create reset token failed: %v", err)
	}

	if err := repo.ConsumeAndReset(ctx, rawToken, "NewPassword1!"); err != nil {
		t.Fatalf("consume and reset failed: %v", err)
	}

	var storedHash string
	err = pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1::uuid`, u.ID).Scan(&storedHash)
	if err != nil {
		t.Fatalf("read user hash failed: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte("NewPassword1!")) != nil {
		t.Fatal("password hash was not updated to the new password")
	}

	var revoked int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE user_id = $1::uuid AND is_revoked = true`, u.ID).Scan(&revoked)
	if err != nil {
		t.Fatalf("count revoked sessions failed: %v", err)
	}
	if revoked != 1 {
		t.Fatalf("revoked sessions=%d want=1", revoked)
	}

	var usedCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM password_reset_tokens
		WHERE token_hash = $1 AND used_at IS NOT NULL
	`, hashToken(rawToken)).Scan(&usedCount)
	if err != nil {
		t.Fatalf("read token usage failed: %v", err)
	}
	if usedCount != 1 {
		t.Fatalf("used token count=%d want=1", usedCount)
	}
}

func TestRepositoryConsumeAndResetTokenStates(t *testing.T) {
	repo, userRepo, pool, ctx := setupResetRepo(t)
	defer pool.Close()

	u, err := userRepo.Create(ctx, "reset-states@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	err = repo.ConsumeAndReset(ctx, "does-not-exist", "new-pass")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing token err=%v want=%v", err, ErrNotFound)
	}

	raw, err := generateToken()
	if err != nil {
		t.Fatalf("generate token failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, $3)
	`, u.ID, hashToken(raw), time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("insert expired token failed: %v", err)
	}

	err = repo.ConsumeAndReset(ctx, raw, "new-pass")
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("expired token err=%v want=%v", err, ErrExpired)
	}
}

// TestRepositoryConsumeAndReset_InvalidTokenSkipsBcrypt proves an unknown token
// is rejected without ever running bcrypt on the caller-supplied password —
// bcrypt.GenerateFromPassword rejects passwords over 72 bytes, so a bcrypt
// attempt on this input would surface as an unexpected error/panic rather than
// the expected ErrNotFound. (ISSUE_LIST #84)
func TestRepositoryConsumeAndReset_InvalidTokenSkipsBcrypt(t *testing.T) {
	repo, _, pool, ctx := setupResetRepo(t)
	defer pool.Close()

	oversizedPassword := string(make([]byte, 100))
	err := repo.ConsumeAndReset(ctx, "does-not-exist-either", oversizedPassword)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid token with oversized password err=%v want=%v", err, ErrNotFound)
	}
}

// TestRepositoryConsumeAndReset_ValidTokenRunsBcrypt proves that once a token
// is confirmed valid, an oversized (>72 byte) password does reach bcrypt and
// surfaces its error, confirming bcrypt only runs post-validation.
// (ISSUE_LIST #84)
func TestRepositoryConsumeAndReset_ValidTokenRunsBcrypt(t *testing.T) {
	repo, userRepo, pool, ctx := setupResetRepo(t)
	defer pool.Close()

	u, err := userRepo.Create(ctx, "reset-oversized@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	rawToken, err := repo.Create(ctx, u.ID)
	if err != nil {
		t.Fatalf("create reset token failed: %v", err)
	}

	oversizedPassword := string(make([]byte, 100))
	err = repo.ConsumeAndReset(ctx, rawToken, oversizedPassword)
	if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrExpired) || errors.Is(err, ErrUsed) {
		t.Fatalf("expected a bcrypt error for an oversized password on a valid token, got %v", err)
	}
}

func TestRepositoryConsumeAndResetReuseForbidden(t *testing.T) {
	repo, userRepo, pool, ctx := setupResetRepo(t)
	defer pool.Close()

	password := "SecretPassword123!"
	oldHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}

	u, err := userRepo.Create(ctx, "reset-reuse@example.com", string(oldHash), "en")
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	rawToken, err := repo.Create(ctx, u.ID)
	if err != nil {
		t.Fatalf("create reset token failed: %v", err)
	}

	err = repo.ConsumeAndReset(ctx, rawToken, password)
	if !errors.Is(err, ErrPasswordReuseForbidden) {
		t.Fatalf("expected ErrPasswordReuseForbidden, got %v", err)
	}
}

package session_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/session"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/internal/user"
)

type integrationUserReader struct {
	byID map[string]*user.User
}

func (r *integrationUserReader) FindByEmail(ctx context.Context, email string) (*user.User, error) {
	return nil, user.ErrNotFound
}

func (r *integrationUserReader) FindByID(ctx context.Context, id string) (*user.User, error) {
	u, ok := r.byID[id]
	if !ok {
		return nil, user.ErrNotFound
	}
	return u, nil
}

func (r *integrationUserReader) CheckPassword(hash, password string) bool {
	return false
}

func (r *integrationUserReader) GetPermissions(ctx context.Context, userID string) ([]string, error) {
	return []string{"sessions:read"}, nil
}

type integrationServiceAccountStore struct{}

func (s *integrationServiceAccountStore) FindByClientID(ctx context.Context, clientID string) (*auth.ServiceAccountRecord, error) {
	return nil, errors.New("not used in refresh integration tests")
}

func (s *integrationServiceAccountStore) CheckSecret(hash, secret string) bool {
	return false
}

func integrationDatabaseURL(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}
	return testdb.URL(t)
}

func setupAuthIntegrationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	dsn := integrationDatabaseURL(t)
	schema := fmt.Sprintf("auth_it_%d", time.Now().UnixNano())

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("integration database unavailable: %v", err)
		return nil
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("integration database unreachable: %v", err)
		return nil
	}
	if _, err := admin.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS "pgcrypto"`); err != nil {
		admin.Close()
		t.Fatalf("create pgcrypto extension: %v", err)
	}
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	admin.Close()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect test schema: %v", err)
	}
	createAuthIntegrationSchema(t, db)

	t.Cleanup(func() {
		db.Close()
		admin, err := pgxpool.New(context.Background(), dsn)
		if err == nil {
			_, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
			admin.Close()
		}
	})
	return db
}

func createAuthIntegrationSchema(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		CREATE TABLE users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) UNIQUE NOT NULL,
			email_hash VARCHAR(64) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			locale VARCHAR(10) NOT NULL DEFAULT 'en',
			is_active BOOLEAN NOT NULL DEFAULT true
		);
		CREATE TABLE sessions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			refresh_token_hash VARCHAR(255) NOT NULL,
			device_info JSONB,
			ip_address INET,
			user_agent TEXT,
			is_revoked BOOLEAN NOT NULL DEFAULT false,
			expires_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			last_used_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX idx_sessions_user_id ON sessions(user_id);
		CREATE INDEX idx_sessions_token_hash ON sessions(refresh_token_hash);
	`)
	if err != nil {
		t.Fatalf("create integration schema tables: %v", err)
	}
}

func createIntegrationUser(t *testing.T, db *pgxpool.Pool, email string) *user.User {
	t.Helper()
	u := &user.User{
		Email:    email,
		Locale:   "en",
		Roles:    []string{"viewer"},
		IsActive: true,
	}
	err := db.QueryRow(context.Background(), `
		INSERT INTO users (email, email_hash, password_hash, locale, is_active)
		VALUES ($1, $2, 'hash', 'en', true)
		RETURNING id
	`, email, hashRefreshToken(email)).Scan(&u.ID)
	if err != nil {
		t.Fatalf("create integration user: %v", err)
	}
	return u
}

func newIntegrationAuthService(t *testing.T, u *user.User, sessions auth.SessionStore) *auth.Service {
	t.Helper()
	ks, err := auth.LoadOrGenerateKeyStore("", "")
	if err != nil {
		t.Fatalf("keystore init failed: %v", err)
	}
	return auth.NewService(
		&integrationUserReader{byID: map[string]*user.User{u.ID: u}},
		sessions,
		&integrationServiceAccountStore{},
		nil,
		ks,
		nil,
		nil,
	)
}

func hashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func countActiveSessions(t *testing.T, db *pgxpool.Pool, userID string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), `
		SELECT count(*) FROM sessions
		WHERE user_id = $1 AND is_revoked = false AND expires_at > now()
	`, userID).Scan(&count); err != nil {
		t.Fatalf("count active sessions: %v", err)
	}
	return count
}

func activeSessionHashes(t *testing.T, db *pgxpool.Pool, userID string) map[string]bool {
	t.Helper()
	rows, err := db.Query(context.Background(), `
		SELECT refresh_token_hash FROM sessions
		WHERE user_id = $1 AND is_revoked = false AND expires_at > now()
	`, userID)
	if err != nil {
		t.Fatalf("query active sessions: %v", err)
	}
	defer rows.Close()

	hashes := map[string]bool{}
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			t.Fatalf("scan active session hash: %v", err)
		}
		hashes[hash] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate active session hashes: %v", err)
	}
	return hashes
}

func TestRefreshTokens_ConcurrentDatabaseRotationKeepsWinnerSession(t *testing.T) {
	db := setupAuthIntegrationDB(t)
	repo := session.NewRepository(db, session.NewEventHub())
	u := createIntegrationUser(t, db, "race@example.com")
	svc := newIntegrationAuthService(t, u, repo)

	const originalRefresh = "same-refresh-token"
	if err := repo.Create(context.Background(), u.ID, hashRefreshToken(originalRefresh), "127.0.0.1", "ua", nil, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create original session: %v", err)
	}

	type result struct {
		pair *auth.TokenPair
		err  error
	}
	results := make(chan result, 2)
	var start sync.WaitGroup
	start.Add(1)
	for i := 0; i < 2; i++ {
		go func() {
			start.Wait()
			pair, err := svc.RefreshTokens(context.Background(), originalRefresh, "127.0.0.1", "ua", nil)
			results <- result{pair: pair, err: err}
		}()
	}
	start.Done()

	got := []result{<-results, <-results}
	successes := 0
	failures := 0
	var winner *auth.TokenPair
	for _, res := range got {
		if res.err == nil {
			successes++
			winner = res.pair
			continue
		}
		if errors.Is(res.err, auth.ErrInvalidToken) {
			failures++
			continue
		}
		t.Fatalf("unexpected refresh error: %v", res.err)
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("want one successful rotation and one invalid-token loser, got successes=%d failures=%d", successes, failures)
	}
	if winner == nil || winner.RefreshToken == "" {
		t.Fatalf("winner token pair missing refresh token: %#v", winner)
	}
	if count := countActiveSessions(t, db, u.ID); count != 1 {
		t.Fatalf("active session count=%d want 1", count)
	}
	if !activeSessionHashes(t, db, u.ID)[hashRefreshToken(winner.RefreshToken)] {
		t.Fatal("winner's rotated refresh token is not the remaining active session")
	}
}

func TestRefreshTokens_RevokedTokenFails(t *testing.T) {
	db := setupAuthIntegrationDB(t)
	repo := session.NewRepository(db, session.NewEventHub())
	u := createIntegrationUser(t, db, "revoked@example.com")
	svc := newIntegrationAuthService(t, u, repo)

	const refresh = "revoked-refresh-token"
	if err := repo.Create(context.Background(), u.ID, hashRefreshToken(refresh), "127.0.0.1", "ua", nil, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.Revoke(context.Background(), hashRefreshToken(refresh)); err != nil {
		t.Fatalf("revoke session: %v", err)
	}

	_, err := svc.RefreshTokens(context.Background(), refresh, "127.0.0.1", "ua", nil)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("RefreshTokens error=%v want ErrInvalidToken", err)
	}
}

func TestRefreshTokens_ReusedRotatedTokenRevokesAllSessions(t *testing.T) {
	db := setupAuthIntegrationDB(t)
	repo := session.NewRepository(db, session.NewEventHub())
	u := createIntegrationUser(t, db, "reuse@example.com")
	svc := newIntegrationAuthService(t, u, repo)

	const originalRefresh = "old-refresh-token"
	if err := repo.Create(context.Background(), u.ID, hashRefreshToken(originalRefresh), "127.0.0.1", "ua", nil, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create original session: %v", err)
	}
	if _, err := svc.RefreshTokens(context.Background(), originalRefresh, "127.0.0.1", "ua", nil); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if err := repo.Create(context.Background(), u.ID, hashRefreshToken("second-active-session"), "127.0.0.2", "ua2", nil, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create second active session: %v", err)
	}
	if count := countActiveSessions(t, db, u.ID); count != 2 {
		t.Fatalf("active session count before reuse=%d want 2", count)
	}
	_, err := db.Exec(context.Background(), `
		UPDATE sessions
		SET last_used_at = now() - ($1::interval)
		WHERE refresh_token_hash = $2
	`, "10 seconds", hashRefreshToken(originalRefresh))
	if err != nil {
		t.Fatalf("age rotated token beyond reuse grace: %v", err)
	}

	_, err = svc.RefreshTokens(context.Background(), originalRefresh, "127.0.0.1", "ua", nil)
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("reused refresh error=%v want ErrInvalidToken", err)
	}
	if count := countActiveSessions(t, db, u.ID); count != 0 {
		t.Fatalf("active session count after token reuse=%d want 0", count)
	}
}

func TestRevokeOtherSessionsKeepsCurrentSessionOnly(t *testing.T) {
	db := setupAuthIntegrationDB(t)
	repo := session.NewRepository(db, session.NewEventHub())
	u := createIntegrationUser(t, db, "others@example.com")

	current := hashRefreshToken("current-refresh-token")
	other1 := hashRefreshToken("other-refresh-token-1")
	other2 := hashRefreshToken("other-refresh-token-2")
	for i, hash := range []string{current, other1, other2} {
		if err := repo.Create(context.Background(), u.ID, hash, fmt.Sprintf("127.0.0.%d", i+1), "ua", nil, time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("create session %d: %v", i, err)
		}
	}

	if err := repo.RevokeOtherSessions(context.Background(), u.ID, current); err != nil {
		t.Fatalf("revoke other sessions: %v", err)
	}
	hashes := activeSessionHashes(t, db, u.ID)
	if len(hashes) != 1 || !hashes[current] {
		t.Fatalf("active hashes after revoke others=%v want only current hash", mapKeys(hashes))
	}
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

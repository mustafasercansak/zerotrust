package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/database"
)

func setupTestDB(t *testing.T) (*pgxpool.Pool, context.Context, *Repository, *user.Repository) {
	t.Helper()
	ctx := context.Background()
	dsn := testdb.URL(t)

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
	if _, err := repo.SecurityDashboard(ctx, "7d"); err == nil {
		t.Fatal("expected security dashboard error after pool close")
	}
}

func TestRepository_SecurityDashboard(t *testing.T) {
	pool, ctx, repo, userRepo := setupTestDB(t)
	defer pool.Close()
	uid := seedAuditUser(t, ctx, userRepo, "security-dashboard@example.com")

	entries := []Entry{
		{
			UserID:    &uid,
			Action:    "auth.login_success",
			IPAddress: "8.8.8.8",
			Metadata: map[string]any{
				"outcome":  "success",
				"location": map[string]any{"country": "United States", "city": "Mountain View"},
			},
		},
		{
			UserID:    &uid,
			Action:    "auth.login_failed",
			IPAddress: "203.0.113.10",
			Metadata:  map[string]any{"outcome": "failure", "reason": "invalid_credentials"},
		},
		{
			UserID:    &uid,
			Action:    "auth.login_failed",
			IPAddress: "203.0.113.10",
			Metadata:  map[string]any{"outcome": "failure", "reason": "account_locked"},
		},
		{
			UserID:    &uid,
			Action:    "login.anomaly",
			IPAddress: "8.8.8.8",
			Metadata:  map[string]any{"outcome": "success", "anomaly_type": "new_device"},
		},
	}
	for _, entry := range entries {
		if err := repo.Log(ctx, entry); err != nil {
			t.Fatalf("log dashboard entry: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, expires_at)
		VALUES ($1, 'active-dashboard-session', NOW() + INTERVAL '1 hour')
	`, uid); err != nil {
		t.Fatalf("insert active session: %v", err)
	}

	result, err := repo.SecurityDashboard(ctx, "7d")
	if err != nil {
		t.Fatalf("SecurityDashboard failed: %v", err)
	}
	if result.Range != "7d" || len(result.AuthActivity) != 7 {
		t.Fatalf("unexpected range data: %+v", result)
	}
	if result.Metrics.SuccessfulLogins != 1 || result.Metrics.FailedLogins != 2 ||
		result.Metrics.Lockouts != 1 || result.Metrics.Anomalies != 1 ||
		result.Metrics.ActiveSessions != 1 {
		t.Fatalf("unexpected metrics: %+v", result.Metrics)
	}
	if len(result.AnomalyBreakdown) != 1 || result.AnomalyBreakdown[0].Name != "new_device" {
		t.Fatalf("unexpected anomaly breakdown: %+v", result.AnomalyBreakdown)
	}
	if len(result.LoginCountries) != 1 || result.LoginCountries[0].Name != "United States" {
		t.Fatalf("unexpected countries: %+v", result.LoginCountries)
	}
	if len(result.FailedLoginIPs) != 1 || result.FailedLoginIPs[0].Count != 2 {
		t.Fatalf("unexpected failed login IPs: %+v", result.FailedLoginIPs)
	}

	result24h, err := repo.SecurityDashboard(ctx, "24h")
	if err != nil {
		t.Fatalf("SecurityDashboard 24h failed: %v", err)
	}
	if result24h.Range != "24h" || len(result24h.AuthActivity) != 24 {
		t.Fatalf("unexpected 24h range data: range=%s points=%d", result24h.Range, len(result24h.AuthActivity))
	}

	defaultResult, err := repo.SecurityDashboard(ctx, "invalid")
	if err != nil {
		t.Fatalf("SecurityDashboard default failed: %v", err)
	}
	if defaultResult.Range != "7d" {
		t.Fatalf("default range=%q want=7d", defaultResult.Range)
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

// mockEncrypter is an in-process encrypter used to test the encrypted metadata path.
type mockEncrypter struct{}

func (m *mockEncrypter) EncryptData(_ context.Context, plaintext string) (string, error) {
	return "enc:" + plaintext, nil
}

func (m *mockEncrypter) DecryptData(_ context.Context, ciphertext string) (string, error) {
	if len(ciphertext) > 4 && ciphertext[:4] == "enc:" {
		return ciphertext[4:], nil
	}
	return ciphertext, nil
}

func TestRepository_LogWithGeoIPLocator(t *testing.T) {
	pool, ctx, repo, _ := setupTestDB(t)
	defer pool.Close()

	repo.SetIPLocator(func(ip string) (string, string) {
		if ip == "8.8.8.8" {
			return "United States", "Mountain View"
		}
		return "", ""
	})

	// IP with known location — location stored in metadata
	if err := repo.Log(ctx, Entry{
		Action:    "test.geoip",
		IPAddress: "8.8.8.8",
	}); err != nil {
		t.Fatalf("Log with locator (known IP): %v", err)
	}

	// IP with no location returned — metadata nil branch (no location added)
	if err := repo.Log(ctx, Entry{
		Action:    "test.geoip.unknown",
		IPAddress: "1.2.3.4",
	}); err != nil {
		t.Fatalf("Log with locator (unknown IP): %v", err)
	}

	// Non-empty existing metadata + locator → location merged
	if err := repo.Log(ctx, Entry{
		Action:    "test.geoip.meta",
		IPAddress: "8.8.8.8",
		Metadata:  map[string]any{"outcome": "success"},
	}); err != nil {
		t.Fatalf("Log with locator + existing metadata: %v", err)
	}

	res, err := repo.List(ctx, ListParams{Action: "test.geoip", Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Total != 3 {
		t.Fatalf("expected 3 geoip entries, got %d", res.Total)
	}
	// First entry should have location in metadata
	found := false
	for _, e := range res.Entries {
		if loc, ok := e.Metadata["location"].(map[string]any); ok {
			if loc["country"] == "United States" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected location metadata in at least one entry")
	}
}

func TestRepository_LogWithEncryptedMetadata(t *testing.T) {
	pool, ctx, repo, userRepo := setupTestDB(t)
	defer pool.Close()

	encrypter := &mockEncrypter{}
	repo.SetSecretsClient(encrypter)
	userRepo.SetSecretsClient(encrypter)
	uid := seedAuditUser(t, ctx, userRepo, "encrypted-audit@example.com")

	// Log with all plaintext-preserved keys
	if err := repo.Log(ctx, Entry{
		UserID: &uid,
		Action: "test.enc",
		Metadata: map[string]any{
			"outcome": "success",
			"status":  200,
			"reason":  "ok",
		},
	}); err != nil {
		t.Fatalf("Log with secClient: %v", err)
	}

	// Log with only sensitive metadata (no outcome/status/location)
	if err := repo.Log(ctx, Entry{
		Action: "test.enc.sensitive",
		Metadata: map[string]any{
			"reason": "internal",
		},
	}); err != nil {
		t.Fatalf("Log with secClient (sensitive only): %v", err)
	}

	// Log with GeoIP + encryption
	repo.SetIPLocator(func(ip string) (string, string) { return "US", "NYC" })
	if err := repo.Log(ctx, Entry{
		Action:    "test.enc.geo",
		IPAddress: "8.8.8.8",
		Metadata:  map[string]any{"outcome": "failure"},
	}); err != nil {
		t.Fatalf("Log with secClient + locator: %v", err)
	}

	// Read back and verify decryption works
	res, err := repo.List(ctx, ListParams{Action: "test.enc", Limit: 10})
	if err != nil {
		t.Fatalf("List encrypted entries: %v", err)
	}
	if res.Total < 2 {
		t.Fatalf("expected at least 2 encrypted entries, got %d", res.Total)
	}
	for _, e := range res.Entries {
		if _, hasPayload := e.Metadata["payload"]; hasPayload {
			t.Fatalf("payload key should be decrypted and removed, got %+v", e.Metadata)
		}
		if e.UserID != nil && (e.UserEmail == nil || *e.UserEmail != "encrypted-audit@example.com") {
			t.Fatalf("user email=%v want=encrypted-audit@example.com", e.UserEmail)
		}
	}
}

func TestRepository_ListRejectsMalformedEncryptedPayload(t *testing.T) {
	pool, ctx, repo, _ := setupTestDB(t)
	defer pool.Close()

	repo.SetSecretsClient(&mockEncrypter{})
	if _, err := pool.Exec(ctx, `
		INSERT INTO audit_logs (action, metadata)
		VALUES ('test.malformed', '{"payload":42}'::jsonb)
	`); err != nil {
		t.Fatalf("insert malformed audit entry: %v", err)
	}

	if _, err := repo.List(ctx, ListParams{Action: "test.malformed", Limit: 10}); err == nil {
		t.Fatal("expected malformed encrypted payload to fail")
	}
}

func TestRepository_ListFilters(t *testing.T) {
	pool, ctx, repo, userRepo := setupTestDB(t)
	defer pool.Close()

	uid := seedAuditUser(t, ctx, userRepo, "filter-test@example.com")

	entries := []Entry{
		{UserID: &uid, Action: "auth.login", Resource: "/api/auth", Metadata: map[string]any{"outcome": "success"}},
		{UserID: &uid, Action: "auth.logout", Resource: "/api/auth", Metadata: map[string]any{"outcome": "success"}},
		{Action: "admin.create", Resource: "/api/admin"},
	}
	for _, e := range entries {
		if err := repo.Log(ctx, e); err != nil {
			t.Fatalf("Log: %v", err)
		}
	}

	cases := []struct {
		name   string
		params ListParams
		want   int
	}{
		{"action filter", ListParams{Action: "auth", Limit: 10}, 2},
		{"resource filter", ListParams{Resource: "/api/admin", Limit: 10}, 1},
		{"user_id filter", ListParams{UserID: uid, Limit: 10}, 2},
		{"outcome filter", ListParams{Outcome: "success", Limit: 10}, 2},
		{"combined filters", ListParams{Action: "auth", UserID: uid, Limit: 10}, 2},
		{"asc sort", ListParams{Limit: 10, SortDir: "asc", SortBy: "action"}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := repo.List(ctx, tc.params)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if res.Total != tc.want {
				t.Fatalf("total=%d want=%d", res.Total, tc.want)
			}
		})
	}
}

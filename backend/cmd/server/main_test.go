package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zerotrust/backend/internal/audit"
	"github.com/zerotrust/backend/internal/serviceaccount"
	"github.com/zerotrust/backend/internal/testdb"
)

func setIntegrationRedisEnv(t *testing.T) {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	password, configured := os.LookupEnv("TEST_REDIS_PASSWORD")
	if !configured {
		password = "61325153d3fbda68c0a7a620e591447fbe75c5dabc93603e"
	}

	t.Setenv("REDIS_ADDR", addr)
	t.Setenv("REDIS_PASSWORD", password)
}

func TestLoadConfig_MFADisabledAllowsMissingOrInvalidKey(t *testing.T) {
	t.Setenv("MFA_ENABLED", "false")
	t.Setenv("MFA_ENCRYPTION_KEY", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig with MFA disabled and missing key: %v", err)
	}
	if cfg.MFAEnabled {
		t.Fatal("MFAEnabled=true want false")
	}
	if len(cfg.MFAEncryptionKey) != 0 {
		t.Fatalf("MFAEncryptionKey length=%d want 0", len(cfg.MFAEncryptionKey))
	}

	t.Setenv("MFA_ENCRYPTION_KEY", "not-hex")
	cfg, err = loadConfig()
	if err != nil {
		t.Fatalf("loadConfig with MFA disabled and invalid key: %v", err)
	}
	if len(cfg.MFAEncryptionKey) != 0 {
		t.Fatalf("MFAEncryptionKey length=%d want 0", len(cfg.MFAEncryptionKey))
	}

	t.Setenv("MFA_ENCRYPTION_KEY", strings.Repeat("a", 64))
	cfg, err = loadConfig()
	if err != nil {
		t.Fatalf("loadConfig with MFA disabled and valid key: %v", err)
	}
	if len(cfg.MFAEncryptionKey) != 0 {
		t.Fatalf("MFAEncryptionKey length=%d want 0 when MFA is disabled", len(cfg.MFAEncryptionKey))
	}
}

func TestLoadConfig_MFAEnabledRequiresValidKey(t *testing.T) {
	t.Setenv("MFA_ENABLED", "true")
	t.Setenv("MFA_ENCRYPTION_KEY", "")

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error when MFA is enabled without key")
	}

	t.Setenv("MFA_ENCRYPTION_KEY", "not-hex")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error when MFA is enabled with invalid key")
	}

	t.Setenv("MFA_ENCRYPTION_KEY", strings.Repeat("a", 64))
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig with MFA enabled and valid key: %v", err)
	}
	if !cfg.MFAEnabled {
		t.Fatal("MFAEnabled=false want true")
	}
	if len(cfg.MFAEncryptionKey) != 32 {
		t.Fatalf("MFAEncryptionKey length=%d want 32", len(cfg.MFAEncryptionKey))
	}
}

func TestLoadConfig_MFAEnabledRejectsInvalidFlagValue(t *testing.T) {
	t.Setenv("MFA_ENABLED", "yes")

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error for invalid MFA_ENABLED value")
	}
}

func TestLoadConfig_MFAEnabledAcceptsTrimmedCaseInsensitiveBool(t *testing.T) {
	t.Setenv("MFA_ENABLED", " TRUE ")
	t.Setenv("MFA_ENCRYPTION_KEY", strings.Repeat("a", 64))

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig with trimmed uppercase MFA_ENABLED: %v", err)
	}
	if !cfg.MFAEnabled {
		t.Fatal("MFAEnabled=false want true")
	}
}

func TestWriteMetricsIncludesAuditWriteFailures(t *testing.T) {
	before := audit.WriteFailures()
	audit.RecordWriteFailure()

	rr := httptest.NewRecorder()
	writeMetrics(rr)

	if got := rr.Header().Get("Content-Type"); got != "text/plain; version=0.0.4" {
		t.Fatalf("Content-Type=%q want text/plain; version=0.0.4", got)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "# TYPE zerotrust_audit_write_failures_total counter") {
		t.Fatalf("missing counter type in metrics body: %q", body)
	}
	want := "zerotrust_audit_write_failures_total " + strconv.FormatUint(before+1, 10)
	if !strings.Contains(body, want) {
		t.Fatalf("metrics body=%q want line containing %q", body, want)
	}
}

type cleanupTestKey struct{}

type cleanupProbe struct {
	once    sync.Once
	ctxSeen chan struct{}
}

func (p *cleanupProbe) RevokeStaleInitialSessions(ctx context.Context) (int64, error) {
	p.recordContext(ctx)
	return 0, nil
}

func (p *cleanupProbe) DeleteExpired(ctx context.Context) (int64, error) {
	p.recordContext(ctx)
	return 0, nil
}

func (p *cleanupProbe) recordContext(ctx context.Context) {
	if ctx.Value(cleanupTestKey{}) == "root" {
		p.once.Do(func() { close(p.ctxSeen) })
	}
}

func TestRunSessionCleanupLoopUsesRootContextAndStopsOnCancel(t *testing.T) {
	base := context.WithValue(context.Background(), cleanupTestKey{}, "root")
	ctx, cancel := context.WithCancel(base)
	probe := &cleanupProbe{ctxSeen: make(chan struct{})}
	done := make(chan struct{})

	go func() {
		runSessionCleanupLoop(ctx, probe, time.Millisecond, time.Millisecond)
		close(done)
	}()

	select {
	case <-probe.ctxSeen:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("cleanup loop did not call repository with root context")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup loop did not stop after context cancellation")
	}
}

func TestWaitForBackgroundHonorsContextDeadline(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	if waitForBackground(ctx, &wg) {
		t.Fatal("waitForBackground returned true before WaitGroup completed")
	}

	wg.Done()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if !waitForBackground(ctx, &wg) {
		t.Fatal("waitForBackground returned false after WaitGroup completed")
	}
}

func TestLoadConfig_ConnectionPoolTuning(t *testing.T) {
	t.Setenv("DATABASE_MAX_CONNS", "50")
	t.Setenv("DATABASE_MIN_CONNS", "5")
	t.Setenv("DATABASE_CONN_TIMEOUT", "10s")
	t.Setenv("REDIS_POOL_SIZE", "25")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading config with pool parameters: %v", err)
	}

	if cfg.DatabaseMaxConns != 50 {
		t.Errorf("expected DatabaseMaxConns to be 50, got %d", cfg.DatabaseMaxConns)
	}
	if cfg.DatabaseMinConns != 5 {
		t.Errorf("expected DatabaseMinConns to be 5, got %d", cfg.DatabaseMinConns)
	}
	if cfg.DatabaseConnTimeout != 10*time.Second {
		t.Errorf("expected DatabaseConnTimeout to be 10s, got %v", cfg.DatabaseConnTimeout)
	}
	if cfg.RedisPoolSize != 25 {
		t.Errorf("expected RedisPoolSize to be 25, got %d", cfg.RedisPoolSize)
	}

	// Verify defaults
	t.Setenv("DATABASE_MAX_CONNS", "")
	t.Setenv("DATABASE_MIN_CONNS", "")
	t.Setenv("DATABASE_CONN_TIMEOUT", "")
	t.Setenv("REDIS_POOL_SIZE", "")

	cfg, err = loadConfig()
	if err != nil {
		t.Fatalf("unexpected error loading default config: %v", err)
	}

	if cfg.DatabaseMaxConns != 20 {
		t.Errorf("expected default DatabaseMaxConns to be 20, got %d", cfg.DatabaseMaxConns)
	}
	if cfg.DatabaseMinConns != 2 {
		t.Errorf("expected default DatabaseMinConns to be 2, got %d", cfg.DatabaseMinConns)
	}
	if cfg.DatabaseConnTimeout != 5*time.Second {
		t.Errorf("expected default DatabaseConnTimeout to be 5s, got %v", cfg.DatabaseConnTimeout)
	}
	if cfg.RedisPoolSize != 10 {
		t.Errorf("expected default RedisPoolSize to be 10, got %d", cfg.RedisPoolSize)
	}
}

func TestLoadConfig_InvalidNumericAndDurationEnv(t *testing.T) {
	t.Setenv("MFA_ENABLED", "false")

	t.Run("invalid database max conns", func(t *testing.T) {
		t.Setenv("DATABASE_MAX_CONNS", "not-int")
		if _, err := loadConfig(); err == nil {
			t.Fatal("expected invalid DATABASE_MAX_CONNS error")
		}
		t.Setenv("DATABASE_MAX_CONNS", "")
	})

	t.Run("invalid database min conns", func(t *testing.T) {
		t.Setenv("DATABASE_MIN_CONNS", "not-int")
		if _, err := loadConfig(); err == nil {
			t.Fatal("expected invalid DATABASE_MIN_CONNS error")
		}
		t.Setenv("DATABASE_MIN_CONNS", "")
	})

	t.Run("invalid redis pool size", func(t *testing.T) {
		t.Setenv("REDIS_POOL_SIZE", "not-int")
		if _, err := loadConfig(); err == nil {
			t.Fatal("expected invalid REDIS_POOL_SIZE error")
		}
		t.Setenv("REDIS_POOL_SIZE", "")
	})

	t.Run("invalid database conn timeout", func(t *testing.T) {
		t.Setenv("DATABASE_CONN_TIMEOUT", "not-duration")
		if _, err := loadConfig(); err == nil {
			t.Fatal("expected invalid DATABASE_CONN_TIMEOUT error")
		}
		t.Setenv("DATABASE_CONN_TIMEOUT", "")
	})
}

func TestLoadConfig_ParsesBooleanFlagsAndOrigins(t *testing.T) {
	t.Setenv("MFA_ENABLED", "false")
	t.Setenv("TLS_ENABLED", "true")
	t.Setenv("COOKIES_SECURE", "true")
	t.Setenv("REGISTRATION_ENABLED", "true")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example.com,https://b.example.com")
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8,192.168.0.0/16")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if !cfg.TLSEnabled {
		t.Fatal("TLSEnabled=false want true")
	}
	if !cfg.CookiesSecure {
		t.Fatal("CookiesSecure=false want true")
	}
	if !cfg.RegistrationEnabled {
		t.Fatal("RegistrationEnabled=false want true")
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[0] != "https://a.example.com" || cfg.CORSOrigins[1] != "https://b.example.com" {
		t.Fatalf("unexpected CORS origins: %v", cfg.CORSOrigins)
	}
	if cfg.TrustedProxies != "10.0.0.0/8,192.168.0.0/16" {
		t.Fatalf("TrustedProxies=%q want=10.0.0.0/8,192.168.0.0/16", cfg.TrustedProxies)
	}
}

func TestLoadConfig_MFAEnabledRejectsWrongKeyLength(t *testing.T) {
	t.Setenv("MFA_ENABLED", "true")
	t.Setenv("MFA_ENCRYPTION_KEY", "aa")

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error for invalid MFA key length")
	}
}

func TestSkipPathsBypassesWrappedMiddlewareForConfiguredPaths(t *testing.T) {
	mwCalled := false
	nextCalled := false

	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mwCalled = true
			next.ServeHTTP(w, r)
		})
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	h := skipPaths(mw, "/health")(next)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}
	if mwCalled {
		t.Fatal("expected wrapped middleware to be bypassed for skipped path")
	}
}

func TestSkipPathsAppliesWrappedMiddlewareForOtherPaths(t *testing.T) {
	mwCalled := false
	nextCalled := false

	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mwCalled = true
			next.ServeHTTP(w, r)
		})
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	h := skipPaths(mw, "/health")(next)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil))

	if !nextCalled || !mwCalled {
		t.Fatalf("expected both next and middleware to be called, got next=%v middleware=%v", nextCalled, mwCalled)
	}
}

func TestRunSessionCleanupReturnsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &cleanupProbe{ctxSeen: make(chan struct{})}

	done := make(chan struct{})
	go func() {
		runSessionCleanup(ctx, repo)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runSessionCleanup did not return after context cancellation")
	}
}

type fakeServiceAccountLookup struct {
	findResp       *serviceaccount.ServiceAccount
	findErr        error
	findClientID   string
	checkHash      string
	checkSecret    string
	checkSecretRet bool
}

func (f *fakeServiceAccountLookup) FindByClientID(ctx context.Context, clientID string) (*serviceaccount.ServiceAccount, error) {
	f.findClientID = clientID
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.findResp, nil
}

func (f *fakeServiceAccountLookup) CheckSecret(hash, secret string) bool {
	f.checkHash = hash
	f.checkSecret = secret
	return f.checkSecretRet
}

func TestSAStoreAdapterFindByClientIDMapsFields(t *testing.T) {
	future := time.Now().Add(time.Hour).UTC()
	oldHash := "old-hash"
	lookup := &fakeServiceAccountLookup{findResp: &serviceaccount.ServiceAccount{
		Name:                "Deploy Bot",
		ClientSecretHash:    "new-hash",
		Scopes:              []string{"users:read"},
		IsActive:            true,
		ExpiresAt:           &future,
		OldClientSecretHash: &oldHash,
		OldSecretExpiresAt:  &future,
	}}
	adapter := &saStoreAdapter{svc: lookup}

	record, err := adapter.FindByClientID(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("FindByClientID returned error: %v", err)
	}
	if lookup.findClientID != "svc-1" {
		t.Fatalf("lookup called with clientID=%q want=svc-1", lookup.findClientID)
	}
	if record.Name != "Deploy Bot" || record.ClientSecretHash != "new-hash" || !record.IsActive {
		t.Fatalf("unexpected mapped record: %+v", record)
	}
	if len(record.Scopes) != 1 || record.Scopes[0] != "users:read" {
		t.Fatalf("unexpected mapped scopes: %+v", record.Scopes)
	}
	if record.ExpiresAt == nil || !record.ExpiresAt.Equal(future) {
		t.Fatalf("unexpected mapped ExpiresAt: %v", record.ExpiresAt)
	}
	if record.OldClientSecretHash == nil || *record.OldClientSecretHash != oldHash {
		t.Fatalf("unexpected mapped OldClientSecretHash: %v", record.OldClientSecretHash)
	}
}

func TestSAStoreAdapterCheckSecretDelegates(t *testing.T) {
	lookup := &fakeServiceAccountLookup{checkSecretRet: true}
	adapter := &saStoreAdapter{svc: lookup}

	if ok := adapter.CheckSecret("hash", "secret"); !ok {
		t.Fatal("CheckSecret returned false, want true")
	}
	if lookup.checkHash != "hash" || lookup.checkSecret != "secret" {
		t.Fatalf("unexpected delegated args hash=%q secret=%q", lookup.checkHash, lookup.checkSecret)
	}
}

func TestSAStoreAdapterFindByClientIDPropagatesError(t *testing.T) {
	wantErr := errors.New("lookup failed")
	lookup := &fakeServiceAccountLookup{findErr: wantErr}
	adapter := &saStoreAdapter{svc: lookup}

	_, err := adapter.FindByClientID(context.Background(), "svc-x")
	if !errors.Is(err, wantErr) {
		t.Fatalf("FindByClientID error=%v want=%v", err, wantErr)
	}
}

func TestGetEnvReturnsFallbackAndValue(t *testing.T) {
	t.Setenv("ZT_TEST_ENV", "")
	if got := getEnv("ZT_TEST_ENV", "fallback"); got != "fallback" {
		t.Fatalf("getEnv fallback=%q want=fallback", got)
	}

	t.Setenv("ZT_TEST_ENV", "value")
	if got := getEnv("ZT_TEST_ENV", "fallback"); got != "value" {
		t.Fatalf("getEnv value=%q want=value", got)
	}
}

func TestBoolEnv(t *testing.T) {
	t.Setenv("ZT_BOOL", "")
	if got, err := boolEnv("ZT_BOOL", true); err != nil || !got {
		t.Fatalf("boolEnv fallback got=%v err=%v", got, err)
	}

	t.Setenv("ZT_BOOL", " FALSE ")
	if got, err := boolEnv("ZT_BOOL", true); err != nil || got {
		t.Fatalf("boolEnv false got=%v err=%v", got, err)
	}

	t.Setenv("ZT_BOOL", "not-bool")
	if _, err := boolEnv("ZT_BOOL", false); err == nil {
		t.Fatal("expected boolEnv to return error for invalid value")
	}
}

func TestIntEnv(t *testing.T) {
	t.Setenv("ZT_INT", "")
	if got, err := intEnv("ZT_INT", 42); err != nil || got != 42 {
		t.Fatalf("intEnv fallback got=%d err=%v", got, err)
	}

	t.Setenv("ZT_INT", " 7 ")
	if got, err := intEnv("ZT_INT", 42); err != nil || got != 7 {
		t.Fatalf("intEnv parse got=%d err=%v", got, err)
	}

	t.Setenv("ZT_INT", "oops")
	if _, err := intEnv("ZT_INT", 42); err == nil {
		t.Fatal("expected intEnv to return error for invalid value")
	}
}

type cleanupCounterProbe struct {
	staleCount int
	purgeCount int
	staleSeen  chan struct{}
	purgeSeen  chan struct{}
}

func (p *cleanupCounterProbe) RevokeStaleInitialSessions(ctx context.Context) (int64, error) {
	p.staleCount++
	if p.staleCount == 1 {
		close(p.staleSeen)
	}
	return 1, nil
}

func (p *cleanupCounterProbe) DeleteExpired(ctx context.Context) (int64, error) {
	p.purgeCount++
	if p.purgeCount == 1 {
		close(p.purgeSeen)
	}
	return 1, nil
}

func TestRunSessionCleanupLoopRunsBothTickers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	probe := &cleanupCounterProbe{
		staleSeen: make(chan struct{}),
		purgeSeen: make(chan struct{}),
	}
	done := make(chan struct{})

	go func() {
		runSessionCleanupLoop(ctx, probe, time.Millisecond, time.Millisecond)
		close(done)
	}()

	select {
	case <-probe.staleSeen:
	case <-time.After(time.Second):
		t.Fatal("stale cleanup ticker did not run")
	}

	select {
	case <-probe.purgeSeen:
	case <-time.After(time.Second):
		t.Fatal("purge cleanup ticker did not run")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup loop did not stop after cancel")
	}
}

func TestRun_BadDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://bad:bad@127.0.0.1:19999/bad?sslmode=disable")
	t.Setenv("MIGRATIONS_PATH", "../../migrations")
	setIntegrationRedisEnv(t)
	t.Setenv("MFA_ENABLED", "false")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := run(ctx, cfg); err == nil {
		t.Fatal("expected error from bad database URL, got nil")
	}
}

func TestRun_BadRedisAddr(t *testing.T) {
	dbURL := testdb.URL(t)

	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("MIGRATIONS_PATH", "../../migrations")
	t.Setenv("REDIS_ADDR", "127.0.0.1:19999")
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("MFA_ENABLED", "false")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := run(ctx, cfg); err == nil {
		t.Fatal("expected error from bad redis addr, got nil")
	}
}

func TestRun_WithMFAAndSMTPAndAdminSeed(t *testing.T) {
	dbURL := testdb.URL(t)

	addr := "127.0.0.1:18766"

	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("MIGRATIONS_PATH", "../../migrations")
	t.Setenv("SERVER_ADDR", addr)
	setIntegrationRedisEnv(t)
	t.Setenv("MFA_ENABLED", "true")
	t.Setenv("MFA_ENCRYPTION_KEY", strings.Repeat("ab", 32)) // 64-char hex
	t.Setenv("REGISTRATION_ENABLED", "false")
	t.Setenv("COOKIES_SECURE", "false")
	t.Setenv("CORS_ORIGINS", "http://localhost:3000")
	t.Setenv("TLS_ENABLED", "false")
	t.Setenv("GEOIP_DB_PATH", "")
	// Seed admin with a pre-hashed password (bcrypt of "Admin1234!")
	t.Setenv("INITIAL_ADMIN_EMAIL", "test-run-admin@example.com")
	t.Setenv("INITIAL_ADMIN_PASSWORD_HASH", "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() { runErr <- run(ctx, cfg) }()

	client := &http.Client{Timeout: time.Second}
	var resp *http.Response
	for i := 0; i < 40; i++ {
		time.Sleep(100 * time.Millisecond)
		resp, err = client.Get("http://" + addr + "/health")
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("server did not become ready: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health check status=%d want=200", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func TestRun_ServerStartsAndResponds(t *testing.T) {
	dbURL := testdb.URL(t)

	// Pick an ephemeral port so multiple test runs don't collide
	addr := "127.0.0.1:18765"

	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("MIGRATIONS_PATH", "../../migrations")
	t.Setenv("SERVER_ADDR", addr)
	setIntegrationRedisEnv(t)
	t.Setenv("MFA_ENABLED", "false")
	t.Setenv("REGISTRATION_ENABLED", "true")
	t.Setenv("COOKIES_SECURE", "false")
	t.Setenv("CORS_ORIGINS", "http://localhost:3000")
	t.Setenv("TLS_ENABLED", "false")
	t.Setenv("GEOIP_DB_PATH", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() { runErr <- run(ctx, cfg) }()

	client := &http.Client{Timeout: time.Second}
	var resp *http.Response
	for i := 0; i < 40; i++ {
		time.Sleep(100 * time.Millisecond)
		resp, err = client.Get("http://" + addr + "/health")
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("server did not become ready: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health check status=%d want=200", resp.StatusCode)
	}

	metricsResp, err := client.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("metrics endpoint: %v", err)
	}
	defer metricsResp.Body.Close()
	if metricsResp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status=%d want=200", metricsResp.StatusCode)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func TestMain_ExitsOnInvalidConfig(t *testing.T) {
	if os.Getenv("ZT_MAIN_HELPER") == "1" {
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMain_ExitsOnInvalidConfig")
	cmd.Env = append(os.Environ(),
		"ZT_MAIN_HELPER=1",
		"MFA_ENABLED=not-bool",
	)

	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected process exit error, got %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code=%d want=1", exitErr.ExitCode())
	}
}

func TestMain_ExitsWhenRunFails(t *testing.T) {
	if os.Getenv("ZT_MAIN_HELPER") == "1" {
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMain_ExitsWhenRunFails")
	cmd.Env = append(os.Environ(),
		"ZT_MAIN_HELPER=1",
		"MFA_ENABLED=false",
		"DATABASE_URL=postgres://bad:bad@127.0.0.1:19999/bad?sslmode=disable",
		"MIGRATIONS_PATH=../../migrations",
		"REDIS_ADDR=127.0.0.1:6379",
	)

	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected process exit error, got %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code=%d want=1", exitErr.ExitCode())
	}
}

func TestRun_AuthenticatedMeRoutes(t *testing.T) {
	dbURL := testdb.URL(t)

	addr := "127.0.0.1:18767"

	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("MIGRATIONS_PATH", "../../migrations")
	t.Setenv("SERVER_ADDR", addr)
	setIntegrationRedisEnv(t)
	t.Setenv("MFA_ENABLED", "false")
	t.Setenv("REGISTRATION_ENABLED", "true")
	t.Setenv("COOKIES_SECURE", "false")
	t.Setenv("CORS_ORIGINS", "http://localhost:3000")
	t.Setenv("TLS_ENABLED", "false")
	t.Setenv("GEOIP_DB_PATH", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() { runErr <- run(ctx, cfg) }()

	client := &http.Client{Timeout: 3 * time.Second}

	base := "http://" + addr
	for i := 0; i < 50; i++ {
		resp, reqErr := client.Get(base + "/health")
		if reqErr == nil {
			resp.Body.Close()
			break
		}
		if i == 49 {
			t.Fatalf("server did not become ready: %v", reqErr)
		}
		time.Sleep(100 * time.Millisecond)
	}

	registerBody := map[string]any{
		"email":    "me-routes@example.com",
		"password": "StrongPassword123!",
		"locale":   "en",
	}
	regPayload, _ := json.Marshal(registerBody)
	regResp, err := client.Post(base+"/api/v1/auth/register", "application/json", bytes.NewReader(regPayload))
	if err != nil {
		t.Fatalf("register request failed: %v", err)
	}
	regResp.Body.Close()
	if regResp.StatusCode != http.StatusCreated {
		t.Fatalf("register status=%d want=%d", regResp.StatusCode, http.StatusCreated)
	}

	loginBody := map[string]any{
		"email":    "me-routes@example.com",
		"password": "StrongPassword123!",
	}
	loginPayload, _ := json.Marshal(loginBody)
	loginResp, err := client.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(loginPayload))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status=%d want=%d", loginResp.StatusCode, http.StatusOK)
	}

	var accessToken string
	for _, c := range loginResp.Cookies() {
		if c.Name == "access_token" {
			accessToken = c.Value
			break
		}
	}
	if accessToken == "" {
		t.Fatal("missing access_token cookie after login")
	}

	newAuthedRequest := func(method, path, body string) *http.Request {
		req, reqErr := http.NewRequest(method, base+path, strings.NewReader(body))
		if reqErr != nil {
			t.Fatalf("new request %s %s: %v", method, path, reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	meResp, err := client.Do(newAuthedRequest(http.MethodGet, "/api/v1/me", ""))
	if err != nil {
		t.Fatalf("GET /me failed: %v", err)
	}
	var mePayload map[string]any
	if err := json.NewDecoder(meResp.Body).Decode(&mePayload); err != nil {
		meResp.Body.Close()
		t.Fatalf("decode /me payload failed: %v", err)
	}
	meResp.Body.Close()
	if meResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /me status=%d want=%d", meResp.StatusCode, http.StatusOK)
	}
	userID, _ := mePayload["user_id"].(string)
	if userID == "" {
		t.Fatal("GET /me returned empty user_id")
	}

	jwksResp, err := client.Get(base + "/.well-known/jwks.json")
	if err != nil {
		t.Fatalf("GET /.well-known/jwks.json failed: %v", err)
	}
	jwksResp.Body.Close()
	if jwksResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /.well-known/jwks.json status=%d want=%d", jwksResp.StatusCode, http.StatusOK)
	}

	profileReq := newAuthedRequest(http.MethodPatch, "/api/v1/me/profile", `{"first_name":"Jane","last_name":"Doe"}`)
	profileResp, err := client.Do(profileReq)
	if err != nil {
		t.Fatalf("PATCH /me/profile failed: %v", err)
	}
	profileResp.Body.Close()
	if profileResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH /me/profile status=%d want=%d", profileResp.StatusCode, http.StatusOK)
	}

	localeBadReq := newAuthedRequest(http.MethodPatch, "/api/v1/me/locale", `{"locale":"de"}`)
	localeBadResp, err := client.Do(localeBadReq)
	if err != nil {
		t.Fatalf("PATCH /me/locale invalid failed: %v", err)
	}
	localeBadResp.Body.Close()
	if localeBadResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH /me/locale invalid status=%d want=%d", localeBadResp.StatusCode, http.StatusBadRequest)
	}

	localeReq := newAuthedRequest(http.MethodPatch, "/api/v1/me/locale", `{"locale":"tr"}`)
	localeResp, err := client.Do(localeReq)
	if err != nil {
		t.Fatalf("PATCH /me/locale failed: %v", err)
	}
	localeResp.Body.Close()
	if localeResp.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH /me/locale status=%d want=%d", localeResp.StatusCode, http.StatusNoContent)
	}

	mfaResp, err := client.Do(newAuthedRequest(http.MethodGet, "/api/v1/mfa/status", ""))
	if err != nil {
		t.Fatalf("GET /mfa/status failed: %v", err)
	}
	mfaResp.Body.Close()
	if mfaResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /mfa/status status=%d want=%d", mfaResp.StatusCode, http.StatusOK)
	}

	mfaChallengeResp, err := client.Post(base+"/api/v1/auth/mfa/challenge", "application/json", strings.NewReader(`{"mfa_token":"","totp_code":""}`))
	if err != nil {
		t.Fatalf("POST /auth/mfa/challenge failed: %v", err)
	}
	mfaChallengeResp.Body.Close()
	if mfaChallengeResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /auth/mfa/challenge status=%d want=%d", mfaChallengeResp.StatusCode, http.StatusBadRequest)
	}

	tokenUnsupportedResp, err := client.Post(base+"/api/v1/auth/token", "application/json", strings.NewReader(`{"grant_type":"password"}`))
	if err != nil {
		t.Fatalf("POST /auth/token unsupported grant failed: %v", err)
	}
	tokenUnsupportedResp.Body.Close()
	if tokenUnsupportedResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /auth/token unsupported grant status=%d want=%d", tokenUnsupportedResp.StatusCode, http.StatusBadRequest)
	}

	tokenMissingFieldsResp, err := client.Post(base+"/api/v1/auth/token", "application/json", strings.NewReader(`{"grant_type":"client_credentials"}`))
	if err != nil {
		t.Fatalf("POST /auth/token missing fields failed: %v", err)
	}
	tokenMissingFieldsResp.Body.Close()
	if tokenMissingFieldsResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /auth/token missing fields status=%d want=%d", tokenMissingFieldsResp.StatusCode, http.StatusBadRequest)
	}

	refreshResp, err := client.Post(base+"/api/v1/auth/refresh", "application/json", strings.NewReader(`{"client_info":{"ua":"test"}}`))
	if err != nil {
		t.Fatalf("POST /auth/refresh failed: %v", err)
	}
	refreshResp.Body.Close()
	if refreshResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /auth/refresh status=%d want=%d", refreshResp.StatusCode, http.StatusBadRequest)
	}

	forgotResp, err := client.Post(base+"/api/v1/auth/forgot-password", "application/json", strings.NewReader(`{"email":"me-routes@example.com"}`))
	if err != nil {
		t.Fatalf("POST /auth/forgot-password failed: %v", err)
	}
	forgotResp.Body.Close()
	if forgotResp.StatusCode != http.StatusOK {
		t.Fatalf("POST /auth/forgot-password status=%d want=%d", forgotResp.StatusCode, http.StatusOK)
	}

	resetMissingResp, err := client.Post(base+"/api/v1/auth/reset-password", "application/json", strings.NewReader(`{"token":"","password":"StrongPassword123!"}`))
	if err != nil {
		t.Fatalf("POST /auth/reset-password missing token failed: %v", err)
	}
	resetMissingResp.Body.Close()
	if resetMissingResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /auth/reset-password missing token status=%d want=%d", resetMissingResp.StatusCode, http.StatusBadRequest)
	}

	logoutResp, err := client.Post(base+"/api/v1/auth/logout", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /auth/logout failed: %v", err)
	}
	logoutResp.Body.Close()
	if logoutResp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /auth/logout status=%d want=%d", logoutResp.StatusCode, http.StatusNoContent)
	}

	var avatarBuf bytes.Buffer
	writer := multipart.NewWriter(&avatarBuf)
	part, err := writer.CreateFormFile("avatar", "avatar.txt")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	if _, err := part.Write([]byte("not-an-image")); err != nil {
		t.Fatalf("write multipart body failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	avatarReq, err := http.NewRequest(http.MethodPost, base+"/api/v1/me/avatar", &avatarBuf)
	if err != nil {
		t.Fatalf("new avatar request failed: %v", err)
	}
	avatarReq.Header.Set("Authorization", "Bearer "+accessToken)
	avatarReq.Header.Set("Content-Type", writer.FormDataContentType())
	avatarResp, err := client.Do(avatarReq)
	if err != nil {
		t.Fatalf("POST /me/avatar failed: %v", err)
	}
	avatarResp.Body.Close()
	if avatarResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /me/avatar status=%d want=%d", avatarResp.StatusCode, http.StatusBadRequest)
	}

	myAvatarResp, err := client.Do(newAuthedRequest(http.MethodGet, "/api/v1/me/avatar", ""))
	if err != nil {
		t.Fatalf("GET /me/avatar failed: %v", err)
	}
	myAvatarResp.Body.Close()
	if myAvatarResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /me/avatar status=%d want=%d", myAvatarResp.StatusCode, http.StatusNotFound)
	}

	userAvatarResp, err := client.Do(newAuthedRequest(http.MethodGet, "/api/v1/users/"+userID+"/avatar", ""))
	if err != nil {
		t.Fatalf("GET /users/{id}/avatar failed: %v", err)
	}
	userAvatarResp.Body.Close()
	if userAvatarResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /users/{id}/avatar status=%d want=%d", userAvatarResp.StatusCode, http.StatusNotFound)
	}

	deleteAvatarReq := newAuthedRequest(http.MethodDelete, "/api/v1/me/avatar", "")
	deleteAvatarResp, err := client.Do(deleteAvatarReq)
	if err != nil {
		t.Fatalf("DELETE /me/avatar failed: %v", err)
	}
	deleteAvatarResp.Body.Close()
	if deleteAvatarResp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /me/avatar status=%d want=%d", deleteAvatarResp.StatusCode, http.StatusOK)
	}

	// PATCH /me/password: missing fields → 400
	pwMissingResp, err := client.Do(newAuthedRequest(http.MethodPatch, "/api/v1/me/password", `{"current_password":"","new_password":""}`))
	if err != nil {
		t.Fatalf("PATCH /me/password missing fields: %v", err)
	}
	pwMissingResp.Body.Close()
	if pwMissingResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH /me/password missing fields status=%d want=%d", pwMissingResp.StatusCode, http.StatusBadRequest)
	}

	// PATCH /me/password: wrong current password → 401
	pwWrongResp, err := client.Do(newAuthedRequest(http.MethodPatch, "/api/v1/me/password", `{"current_password":"WrongPass1!","new_password":"NewStrong456@"}`))
	if err != nil {
		t.Fatalf("PATCH /me/password wrong current: %v", err)
	}
	pwWrongResp.Body.Close()
	if pwWrongResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("PATCH /me/password wrong current status=%d want=%d", pwWrongResp.StatusCode, http.StatusUnauthorized)
	}

	// PATCH /me/password: valid change → 204
	pwOKResp, err := client.Do(newAuthedRequest(http.MethodPatch, "/api/v1/me/password", `{"current_password":"StrongPassword123!","new_password":"NewStrong456@"}`))
	if err != nil {
		t.Fatalf("PATCH /me/password valid: %v", err)
	}
	pwOKResp.Body.Close()
	if pwOKResp.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH /me/password valid status=%d want=%d", pwOKResp.StatusCode, http.StatusNoContent)
	}

	// PATCH /me/notifications: invalid JSON → 400
	notifBadResp, err := client.Do(newAuthedRequest(http.MethodPatch, "/api/v1/me/notifications", `not-json`))
	if err != nil {
		t.Fatalf("PATCH /me/notifications bad JSON: %v", err)
	}
	notifBadResp.Body.Close()
	if notifBadResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PATCH /me/notifications bad JSON status=%d want=%d", notifBadResp.StatusCode, http.StatusBadRequest)
	}

	// PATCH /me/notifications: opt-out → 204
	notifOffResp, err := client.Do(newAuthedRequest(http.MethodPatch, "/api/v1/me/notifications", `{"notify_security_emails":false}`))
	if err != nil {
		t.Fatalf("PATCH /me/notifications false: %v", err)
	}
	notifOffResp.Body.Close()
	if notifOffResp.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH /me/notifications false status=%d want=%d", notifOffResp.StatusCode, http.StatusNoContent)
	}

	// GET /me reflects notify_security_emails=false
	meAfterNotifResp, err := client.Do(newAuthedRequest(http.MethodGet, "/api/v1/me", ""))
	if err != nil {
		t.Fatalf("GET /me after notifications update: %v", err)
	}
	var meAfterNotif map[string]any
	if err := json.NewDecoder(meAfterNotifResp.Body).Decode(&meAfterNotif); err != nil {
		meAfterNotifResp.Body.Close()
		t.Fatalf("decode /me after notifications: %v", err)
	}
	meAfterNotifResp.Body.Close()
	if meAfterNotifResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /me after notifications status=%d want=%d", meAfterNotifResp.StatusCode, http.StatusOK)
	}
	if v, _ := meAfterNotif["notify_security_emails"].(bool); v {
		t.Fatal("GET /me notify_security_emails should be false after opt-out")
	}

	// PATCH /me/notifications: opt back in → 204
	notifOnResp, err := client.Do(newAuthedRequest(http.MethodPatch, "/api/v1/me/notifications", `{"notify_security_emails":true}`))
	if err != nil {
		t.Fatalf("PATCH /me/notifications true: %v", err)
	}
	notifOnResp.Body.Close()
	if notifOnResp.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH /me/notifications true status=%d want=%d", notifOnResp.StatusCode, http.StatusNoContent)
	}

	// Locale is currently "tr" (set earlier in this test). Change to "en" → 204 + audit entry.
	localeChangeResp, err := client.Do(newAuthedRequest(http.MethodPatch, "/api/v1/me/locale", `{"locale":"en"}`))
	if err != nil {
		t.Fatalf("PATCH /me/locale tr→en: %v", err)
	}
	localeChangeResp.Body.Close()
	if localeChangeResp.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH /me/locale tr→en status=%d want=%d", localeChangeResp.StatusCode, http.StatusNoContent)
	}

	// GET /me/audit: verify a locale_changed entry with from=tr to=en exists.
	auditResp, err := client.Do(newAuthedRequest(http.MethodGet, "/api/v1/me/audit", ""))
	if err != nil {
		t.Fatalf("GET /me/audit: %v", err)
	}
	var auditPayload struct {
		Data []struct {
			Action   string         `json:"action"`
			Metadata map[string]any `json:"metadata"`
		} `json:"data"`
	}
	if err := json.NewDecoder(auditResp.Body).Decode(&auditPayload); err != nil {
		auditResp.Body.Close()
		t.Fatalf("decode /me/audit: %v", err)
	}
	auditResp.Body.Close()
	if auditResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /me/audit status=%d want=%d", auditResp.StatusCode, http.StatusOK)
	}
	var foundLocaleChanged bool
	for _, e := range auditPayload.Data {
		if e.Action == "user.locale_changed" && e.Metadata["from"] == "tr" && e.Metadata["to"] == "en" {
			foundLocaleChanged = true
			break
		}
	}
	if !foundLocaleChanged {
		t.Fatal("audit log missing user.locale_changed entry with from=tr to=en")
	}

	// Count how many locale_changed entries exist before the same-locale PATCH.
	var countBeforeSame int
	for _, e := range auditPayload.Data {
		if e.Action == "user.locale_changed" {
			countBeforeSame++
		}
	}

	// PATCH /me/locale: same locale (en→en) → 204, no new audit entry.
	localeSameResp, err := client.Do(newAuthedRequest(http.MethodPatch, "/api/v1/me/locale", `{"locale":"en"}`))
	if err != nil {
		t.Fatalf("PATCH /me/locale en→en: %v", err)
	}
	localeSameResp.Body.Close()
	if localeSameResp.StatusCode != http.StatusNoContent {
		t.Fatalf("PATCH /me/locale en→en status=%d want=%d", localeSameResp.StatusCode, http.StatusNoContent)
	}

	auditResp2, err := client.Do(newAuthedRequest(http.MethodGet, "/api/v1/me/audit", ""))
	if err != nil {
		t.Fatalf("GET /me/audit after same-locale: %v", err)
	}
	var auditPayload2 struct {
		Data []struct {
			Action string `json:"action"`
		} `json:"data"`
	}
	if err := json.NewDecoder(auditResp2.Body).Decode(&auditPayload2); err != nil {
		auditResp2.Body.Close()
		t.Fatalf("decode /me/audit after same-locale: %v", err)
	}
	auditResp2.Body.Close()
	var localeChangedCount int
	for _, e := range auditPayload2.Data {
		if e.Action == "user.locale_changed" {
			localeChangedCount++
		}
	}
	if localeChangedCount != countBeforeSame {
		t.Fatalf("same-locale PATCH must not add audit entry: before=%d after=%d", countBeforeSame, localeChangedCount)
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

func TestRun_ServerListenFailureReturnsError(t *testing.T) {
	dbURL := testdb.URL(t)

	ln, err := net.Listen("tcp", "127.0.0.1:18769")
	if err != nil {
		t.Fatalf("pre-bind listener failed: %v", err)
	}
	defer ln.Close()

	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("MIGRATIONS_PATH", "../../migrations")
	t.Setenv("SERVER_ADDR", "127.0.0.1:18769")
	setIntegrationRedisEnv(t)
	t.Setenv("MFA_ENABLED", "false")
	t.Setenv("REGISTRATION_ENABLED", "true")
	t.Setenv("COOKIES_SECURE", "false")
	t.Setenv("TLS_ENABLED", "false")
	t.Setenv("GEOIP_DB_PATH", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = run(ctx, cfg)
	if err == nil {
		t.Fatal("expected run to fail when listen address is already in use")
	}
}

func TestRun_InvalidBaoAddrFailsSecretsClientInit(t *testing.T) {
	dbURL := testdb.URL(t)

	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("MIGRATIONS_PATH", "../../migrations")
	t.Setenv("SERVER_ADDR", "127.0.0.1:18770")
	setIntegrationRedisEnv(t)
	t.Setenv("MFA_ENABLED", "false")
	t.Setenv("REGISTRATION_ENABLED", "true")
	t.Setenv("COOKIES_SECURE", "false")
	t.Setenv("TLS_ENABLED", "false")
	t.Setenv("GEOIP_DB_PATH", "")
	t.Setenv("BAO_ADDR", "://bad-url")
	t.Setenv("BAO_TOKEN", "test-token")
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = run(ctx, cfg)
	if err == nil {
		t.Fatal("expected run to fail when BAO_ADDR is invalid")
	}
	if !strings.Contains(err.Error(), "failed to initialize secrets client") {
		t.Fatalf("error=%q does not contain expected secrets init failure", err.Error())
	}
}

func TestRun_RefreshAndAvatarSuccessPaths(t *testing.T) {
	dbURL := testdb.URL(t)

	addr := "127.0.0.1:18771"
	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("MIGRATIONS_PATH", "../../migrations")
	t.Setenv("SERVER_ADDR", addr)
	setIntegrationRedisEnv(t)
	t.Setenv("MFA_ENABLED", "false")
	t.Setenv("REGISTRATION_ENABLED", "true")
	t.Setenv("COOKIES_SECURE", "false")
	t.Setenv("CORS_ORIGINS", "http://localhost:3000")
	t.Setenv("TLS_ENABLED", "false")
	t.Setenv("GEOIP_DB_PATH", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- run(ctx, cfg) }()

	client := &http.Client{Timeout: 3 * time.Second}
	base := "http://" + addr
	for i := 0; i < 50; i++ {
		resp, reqErr := client.Get(base + "/health")
		if reqErr == nil {
			resp.Body.Close()
			break
		}
		if i == 49 {
			t.Fatalf("server did not become ready: %v", reqErr)
		}
		time.Sleep(100 * time.Millisecond)
	}

	registerResp, err := client.Post(base+"/api/v1/auth/register", "application/json", strings.NewReader(`{"email":"avatar-flow@example.com","password":"StrongPassword123!","locale":"en"}`))
	if err != nil {
		t.Fatalf("register request failed: %v", err)
	}
	if registerResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(registerResp.Body)
		registerResp.Body.Close()
		t.Fatalf("register status=%d want=%d body=%s", registerResp.StatusCode, http.StatusCreated, string(body))
	}
	registerResp.Body.Close()

	loginResp, err := client.Post(base+"/api/v1/auth/login", "application/json", strings.NewReader(`{"email":"avatar-flow@example.com","password":"StrongPassword123!"}`))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	if loginResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(loginResp.Body)
		loginResp.Body.Close()
		t.Fatalf("login status=%d want=%d body=%s", loginResp.StatusCode, http.StatusOK, string(body))
	}
	loginResp.Body.Close()

	var accessToken, refreshToken, csrfToken string
	for _, c := range loginResp.Cookies() {
		switch c.Name {
		case "access_token":
			accessToken = c.Value
		case "refresh_token":
			refreshToken = c.Value
		case "csrf_token":
			csrfToken = c.Value
		}
	}
	if accessToken == "" || refreshToken == "" || csrfToken == "" {
		t.Fatal("expected access_token, refresh_token and csrf_token cookies from login")
	}

	refreshReq, err := http.NewRequest(http.MethodPost, base+"/api/v1/auth/refresh", strings.NewReader(`{"client_info":{"browser":"itest"}}`))
	if err != nil {
		t.Fatalf("new refresh request: %v", err)
	}
	refreshReq.Header.Set("Content-Type", "application/json")
	refreshReq.Header.Set("X-CSRF-Token", csrfToken)
	refreshReq.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshToken})
	refreshReq.AddCookie(&http.Cookie{Name: "csrf_token", Value: csrfToken})
	refreshResp, err := client.Do(refreshReq)
	if err != nil {
		t.Fatalf("refresh request failed: %v", err)
	}
	if refreshResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(refreshResp.Body)
		refreshResp.Body.Close()
		t.Fatalf("refresh status=%d want=%d body=%s", refreshResp.StatusCode, http.StatusOK, string(body))
	}
	refreshResp.Body.Close()

	authedReq := func(method, path string, body io.Reader) *http.Request {
		req, reqErr := http.NewRequest(method, base+path, body)
		if reqErr != nil {
			t.Fatalf("new request %s %s: %v", method, path, reqErr)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		return req
	}

	meReq := authedReq(http.MethodGet, "/api/v1/me", nil)
	meResp, err := client.Do(meReq)
	if err != nil {
		t.Fatalf("GET /me failed: %v", err)
	}
	if meResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(meResp.Body)
		meResp.Body.Close()
		t.Fatalf("GET /me status=%d want=%d body=%s", meResp.StatusCode, http.StatusOK, string(body))
	}
	var mePayload map[string]any
	if err := json.NewDecoder(meResp.Body).Decode(&mePayload); err != nil {
		meResp.Body.Close()
		t.Fatalf("decode /me payload failed: %v", err)
	}
	meResp.Body.Close()
	userID, _ := mePayload["user_id"].(string)
	if userID == "" {
		t.Fatal("missing user_id in /me payload")
	}

	var avatarBuf bytes.Buffer
	writer := multipart.NewWriter(&avatarBuf)
	part, err := writer.CreateFormFile("avatar", "avatar.png")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	// 1x1 PNG
	pngData := []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 31, 21, 196, 137, 0, 0, 0, 13, 73, 68, 65, 84, 8, 153, 99, 0, 1, 0, 0, 5, 0, 1, 13, 10, 45, 180, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130}
	if _, err := part.Write(pngData); err != nil {
		t.Fatalf("write png data failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	uploadReq := authedReq(http.MethodPost, "/api/v1/me/avatar", &avatarBuf)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		t.Fatalf("POST /me/avatar failed: %v", err)
	}
	if uploadResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(uploadResp.Body)
		uploadResp.Body.Close()
		t.Fatalf("POST /me/avatar status=%d want=%d body=%s", uploadResp.StatusCode, http.StatusOK, string(body))
	}
	uploadResp.Body.Close()

	myAvatarResp, err := client.Do(authedReq(http.MethodGet, "/api/v1/me/avatar", nil))
	if err != nil {
		t.Fatalf("GET /me/avatar failed: %v", err)
	}
	if myAvatarResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(myAvatarResp.Body)
		myAvatarResp.Body.Close()
		t.Fatalf("GET /me/avatar status=%d want=%d body=%s", myAvatarResp.StatusCode, http.StatusOK, string(body))
	}
	myAvatarResp.Body.Close()

	userAvatarResp, err := client.Do(authedReq(http.MethodGet, "/api/v1/users/"+userID+"/avatar", nil))
	if err != nil {
		t.Fatalf("GET /users/{id}/avatar failed: %v", err)
	}
	if userAvatarResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(userAvatarResp.Body)
		userAvatarResp.Body.Close()
		t.Fatalf("GET /users/{id}/avatar status=%d want=%d body=%s", userAvatarResp.StatusCode, http.StatusOK, string(body))
	}
	userAvatarResp.Body.Close()

	deleteResp, err := client.Do(authedReq(http.MethodDelete, "/api/v1/me/avatar", nil))
	if err != nil {
		t.Fatalf("DELETE /me/avatar failed: %v", err)
	}
	if deleteResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(deleteResp.Body)
		deleteResp.Body.Close()
		t.Fatalf("DELETE /me/avatar status=%d want=%d body=%s", deleteResp.StatusCode, http.StatusOK, string(body))
	}
	deleteResp.Body.Close()

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("run returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zerotrust/backend/internal/audit"
	"github.com/zerotrust/backend/internal/serviceaccount"
)

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

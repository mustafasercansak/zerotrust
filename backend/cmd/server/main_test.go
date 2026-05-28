package main

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/zerotrust/backend/internal/audit"
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

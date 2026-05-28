package main

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/zerotrust/backend/internal/audit"
)

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

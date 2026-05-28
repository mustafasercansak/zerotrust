package middleware

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zerotrust/backend/internal/audit"
	"github.com/zerotrust/backend/internal/auth"
)

type recordingAuditLogger struct {
	mu      sync.Mutex
	entries []audit.Entry
	err     error
	done    chan struct{}
}

func newRecordingAuditLogger(err error) *recordingAuditLogger {
	return &recordingAuditLogger{err: err, done: make(chan struct{}, 1)}
}

func (l *recordingAuditLogger) Log(ctx context.Context, entry audit.Entry) error {
	l.mu.Lock()
	l.entries = append(l.entries, entry)
	l.mu.Unlock()
	l.done <- struct{}{}
	return l.err
}

func (l *recordingAuditLogger) wait(t *testing.T) audit.Entry {
	t.Helper()
	select {
	case <-l.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for audit write")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) != 1 {
		t.Fatalf("entries=%d want=1", len(l.entries))
	}
	return l.entries[0]
}

func (l *recordingAuditLogger) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}

func TestAuditLogRecordsCriticalRouteActions(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		action string
	}{
		{name: "role change", method: http.MethodPatch, path: "/api/v1/admin/users/u1/roles", action: "admin.user.roles_update"},
		{name: "user create", method: http.MethodPost, path: "/api/v1/admin/users", action: "admin.user.create"},
		{name: "user status", method: http.MethodPatch, path: "/api/v1/admin/users/u1/status", action: "admin.user.status_update"},
		{name: "self revoke one session", method: http.MethodDelete, path: "/api/v1/sessions/s1", action: "session.revoke"},
		{name: "self revoke other sessions", method: http.MethodDelete, path: "/api/v1/sessions", action: "session.revoke_others"},
		{name: "admin revoke all user sessions", method: http.MethodDelete, path: "/api/v1/admin/users/u1/sessions", action: "admin.user.sessions_revoke_all"},
		{name: "admin revoke one user session", method: http.MethodDelete, path: "/api/v1/admin/users/u1/sessions/s1", action: "admin.user.session_revoke"},
		{name: "settings update", method: http.MethodPatch, path: "/api/v1/admin/settings", action: "admin.settings.update"},
		{name: "service account create", method: http.MethodPost, path: "/api/v1/admin/service-accounts", action: "service_account.create"},
		{name: "service account update", method: http.MethodPatch, path: "/api/v1/admin/service-accounts/sa1", action: "service_account.update"},
		{name: "service account status", method: http.MethodPatch, path: "/api/v1/admin/service-accounts/sa1/status", action: "service_account.status_update"},
		{name: "service account delete", method: http.MethodDelete, path: "/api/v1/admin/service-accounts/sa1", action: "service_account.delete"},
		{name: "mfa setup", method: http.MethodPost, path: "/api/v1/mfa/setup", action: "mfa.setup_start"},
		{name: "mfa verify", method: http.MethodPost, path: "/api/v1/mfa/verify", action: "mfa.verify"},
		{name: "mfa disable", method: http.MethodPost, path: "/api/v1/mfa/disable", action: "mfa.disable"},
		{name: "mfa step up", method: http.MethodPost, path: "/api/v1/mfa/step-up", action: "mfa.step_up"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := newRecordingAuditLogger(nil)
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			handler := AuditLog(logger)(next)

			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.RemoteAddr = "203.0.113.10:1234"
			req.Header.Set("User-Agent", "test-agent")
			claims := &auth.Claims{UserID: "user-1"}
			req = req.WithContext(context.WithValue(req.Context(), ClaimsKey, claims))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)
			entry := logger.wait(t)

			if entry.Action != tt.action {
				t.Fatalf("Action=%q want=%q", entry.Action, tt.action)
			}
			if entry.Resource != tt.path {
				t.Fatalf("Resource=%q want=%q", entry.Resource, tt.path)
			}
			if entry.UserID == nil || *entry.UserID != "user-1" {
				t.Fatalf("UserID=%v want user-1", entry.UserID)
			}
			if entry.IPAddress != "203.0.113.10:1234" {
				t.Fatalf("IPAddress=%q want remote addr", entry.IPAddress)
			}
			if entry.UserAgent != "test-agent" {
				t.Fatalf("UserAgent=%q want test-agent", entry.UserAgent)
			}
			if entry.Metadata["status"] != http.StatusNoContent {
				t.Fatalf("metadata status=%v want %d", entry.Metadata["status"], http.StatusNoContent)
			}
			if entry.Metadata["outcome"] != "success" {
				t.Fatalf("metadata outcome=%v want success", entry.Metadata["outcome"])
			}
		})
	}
}

func TestAuditLogRecordsPublicRequestActions(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		action string
		status int
	}{
		{name: "login", method: http.MethodPost, path: "/api/v1/auth/login", action: "request.auth.login", status: http.StatusBadRequest},
		{name: "mfa challenge", method: http.MethodPost, path: "/api/v1/auth/mfa/challenge", action: "request.auth.mfa_challenge", status: http.StatusUnauthorized},
		{name: "token", method: http.MethodPost, path: "/api/v1/auth/token", action: "request.auth.token", status: http.StatusBadRequest},
		{name: "refresh", method: http.MethodPost, path: "/api/v1/auth/refresh", action: "request.auth.refresh", status: http.StatusBadRequest},
		{name: "logout", method: http.MethodPost, path: "/api/v1/auth/logout", action: "request.auth.logout", status: http.StatusNoContent},
		{name: "register", method: http.MethodPost, path: "/api/v1/auth/register", action: "request.auth.register", status: http.StatusForbidden},
		{name: "forgot password", method: http.MethodPost, path: "/api/v1/auth/forgot-password", action: "request.auth.password_reset_request", status: http.StatusBadRequest},
		{name: "reset password", method: http.MethodPost, path: "/api/v1/auth/reset-password", action: "request.auth.password_reset", status: http.StatusBadRequest},
		{name: "service account events", method: http.MethodGet, path: "/api/v1/admin/service-accounts/events", action: "request.service_account.events", status: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := newRecordingAuditLogger(nil)
			handler := AuditLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)
			entry := logger.wait(t)

			if entry.Action != tt.action {
				t.Fatalf("Action=%q want=%q", entry.Action, tt.action)
			}
			if entry.Metadata["status"] != tt.status {
				t.Fatalf("metadata status=%v want %d", entry.Metadata["status"], tt.status)
			}
		})
	}
}

func TestAuditAuthFailuresRecordsProtected401Attempts(t *testing.T) {
	logger := newRecordingAuditLogger(nil)
	handler := AuditAuthFailures(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/roles", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	entry := logger.wait(t)

	if entry.Action != "admin.user.roles_update" {
		t.Fatalf("Action=%q want admin.user.roles_update", entry.Action)
	}
	if entry.Metadata["status"] != http.StatusUnauthorized {
		t.Fatalf("metadata status=%v want %d", entry.Metadata["status"], http.StatusUnauthorized)
	}
	if entry.Metadata["outcome"] != "failure" {
		t.Fatalf("metadata outcome=%v want failure", entry.Metadata["outcome"])
	}
}

func TestAuditAuthFailuresSkipsNon401Responses(t *testing.T) {
	logger := newRecordingAuditLogger(nil)
	handler := AuditAuthFailures(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/roles", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := logger.count(); got != 0 {
		t.Fatalf("audit entries=%d want=0", got)
	}
}

func TestAuditCSRFFailuresRecordsPreRoute403Attempts(t *testing.T) {
	logger := newRecordingAuditLogger(nil)
	handler := AuditCSRFFailures(logger)(CSRF()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/roles", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "access-token"})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	entry := logger.wait(t)

	if entry.Action != "admin.user.roles_update" {
		t.Fatalf("Action=%q want admin.user.roles_update", entry.Action)
	}
	if entry.Metadata["status"] != http.StatusForbidden {
		t.Fatalf("metadata status=%v want %d", entry.Metadata["status"], http.StatusForbidden)
	}
	if entry.Metadata["outcome"] != "failure" {
		t.Fatalf("metadata outcome=%v want failure", entry.Metadata["outcome"])
	}
	if entry.Metadata["reason"] != "csrf_missing" {
		t.Fatalf("metadata reason=%v want csrf_missing", entry.Metadata["reason"])
	}
}

func TestAuditCSRFFailuresSkipsNonCSRF403Responses(t *testing.T) {
	logger := newRecordingAuditLogger(nil)
	handler := AuditCSRFFailures(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/roles", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := logger.count(); got != 0 {
		t.Fatalf("audit entries=%d want=0", got)
	}

	handler = AuditCSRFFailures(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	rr = httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := logger.count(); got != 0 {
		t.Fatalf("audit entries after non-CSRF 403=%d want=0", got)
	}
}

func TestAuditLogRecordsFailureOutcome(t *testing.T) {
	logger := newRecordingAuditLogger(nil)
	handler := AuditLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/roles", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	entry := logger.wait(t)

	if entry.Metadata["status"] != http.StatusForbidden {
		t.Fatalf("metadata status=%v want %d", entry.Metadata["status"], http.StatusForbidden)
	}
	if entry.Metadata["outcome"] != "failure" {
		t.Fatalf("metadata outcome=%v want failure", entry.Metadata["outcome"])
	}
}

func TestAuditLogPreservesFlusherForStreamingHandlers(t *testing.T) {
	logger := newRecordingAuditLogger(nil)
	handler := AuditLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "missing flusher", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/events", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	logger.wait(t)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !rr.Flushed {
		t.Fatal("expected streaming handler flush to reach recorder")
	}
}

type optionalInterfaceRecorder struct {
	*httptest.ResponseRecorder
	readFromCalled bool
	pushCalled     bool
}

func (r *optionalInterfaceRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.readFromCalled = true
	return io.Copy(r.ResponseRecorder, src)
}

func (r *optionalInterfaceRecorder) Push(target string, opts *http.PushOptions) error {
	r.pushCalled = true
	return nil
}

func (r *optionalInterfaceRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("not connected")
}

func TestAuditLogPreservesOptionalResponseWriterInterfaces(t *testing.T) {
	logger := newRecordingAuditLogger(nil)
	handler := AuditLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(io.ReaderFrom); !ok {
			http.Error(w, "missing readerfrom", http.StatusInternalServerError)
			return
		}
		pusher, ok := w.(http.Pusher)
		if !ok {
			http.Error(w, "missing pusher", http.StatusInternalServerError)
			return
		}
		if _, ok := w.(http.Hijacker); !ok {
			http.Error(w, "missing hijacker", http.StatusInternalServerError)
			return
		}
		_ = pusher.Push("/asset.js", nil)
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rr := &optionalInterfaceRecorder{ResponseRecorder: httptest.NewRecorder()}

	handler.ServeHTTP(rr, req)
	logger.wait(t)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
	if !rr.pushCalled {
		t.Fatal("expected Push to reach underlying writer")
	}
}

func TestAuditLogDoesNotExposeUnsupportedOptionalInterfaces(t *testing.T) {
	logger := newRecordingAuditLogger(nil)
	handler := AuditLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Pusher); ok {
			http.Error(w, "unexpected pusher", http.StatusInternalServerError)
			return
		}
		if _, ok := w.(http.Hijacker); ok {
			http.Error(w, "unexpected hijacker", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	logger.wait(t)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNoContent, rr.Body.String())
	}
}

func TestCriticalAuditRouteIsSynchronous(t *testing.T) {
	logger := newRecordingAuditLogger(nil)
	handler := AuditLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/roles", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := logger.count(); got != 1 {
		t.Fatalf("audit entries after handler return=%d want=1", got)
	}
}

func TestAuditLogWriteFailureIsLogged(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	failuresBefore := audit.WriteFailures()
	logger := newRecordingAuditLogger(errors.New("db down"))
	handler := AuditLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/u1/roles", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	logger.wait(t)

	var logged string
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		logged = buf.String()
		if strings.Contains(logged, "audit log write failed") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(logged, "audit log write failed") {
		t.Fatalf("expected audit failure log, got %q", logged)
	}
	if !strings.Contains(logged, "admin.user.roles_update") {
		t.Fatalf("expected action in log, got %q", logged)
	}
	if got := audit.WriteFailures(); got != failuresBefore+1 {
		t.Fatalf("audit write failures=%d want %d", got, failuresBefore+1)
	}
}

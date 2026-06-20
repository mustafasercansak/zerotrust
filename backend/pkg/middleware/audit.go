package middleware

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/zerotrust/backend/internal/audit"
)

type AuditLogger interface {
	Log(context.Context, audit.Entry) error
}

// auditExtrasKey is the context key for handler-supplied audit metadata.
type auditExtrasKey struct{}

// AuditExtras holds key-value pairs that a handler wants to add to the
// middleware-generated audit log entry for the current request.
type AuditExtras struct {
	data map[string]any
}

func (e *AuditExtras) Set(key string, value any) {
	if e.data == nil {
		e.data = map[string]any{}
	}
	e.data[key] = value
}

// AuditExtrasFrom returns the AuditExtras stored in ctx, or nil if absent.
func AuditExtrasFrom(ctx context.Context) *AuditExtras {
	v, _ := ctx.Value(auditExtrasKey{}).(*AuditExtras)
	return v
}

func AuditLog(repo AuditLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			extras := &AuditExtras{}
			r = r.WithContext(context.WithValue(r.Context(), auditExtrasKey{}, extras))

			rw := newAuditResponseWriter(w)
			next.ServeHTTP(wrapAuditResponseWriter(rw), r)

			claims := ClaimsFrom(r.Context())
			event := auditEventFor(r.Method, r.URL.Path)

			meta := auditMetadata(r, rw.status)
			for k, v := range extras.data {
				meta[k] = v
			}

			entry := audit.Entry{
				Action:    event.action,
				Resource:  event.resource,
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  meta,
			}
			if claims != nil {
				entry.UserID = &claims.UserID
			}

			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			if event.critical {
				defer cancel()
				if err := repo.Log(ctx, entry); err != nil {
					logAuditFailure(err, entry)
				}
				return
			}

			go func() {
				defer cancel()
				if err := repo.Log(ctx, entry); err != nil {
					logAuditFailure(err, entry)
				}
			}()
		})
	}
}

func AuditAuthFailures(repo AuditLogger) func(http.Handler) http.Handler {
	return auditFailures(repo, http.StatusUnauthorized)
}

func AuditCSRFFailures(repo AuditLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := newAuditResponseWriter(w)
			next.ServeHTTP(wrapAuditResponseWriter(rw), r)
			reason := CSRFFailureFrom(r.Context())
			if rw.status != http.StatusForbidden || reason == "" {
				return
			}

			event := auditEventFor(r.Method, r.URL.Path)
			metadata := auditMetadata(r, rw.status)
			metadata["reason"] = reason
			entry := audit.Entry{
				Action:    event.action,
				Resource:  event.resource,
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  metadata,
			}

			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			if event.critical {
				defer cancel()
				if err := repo.Log(ctx, entry); err != nil {
					logAuditFailure(err, entry)
				}
				return
			}
			go func() {
				defer cancel()
				if err := repo.Log(ctx, entry); err != nil {
					logAuditFailure(err, entry)
				}
			}()
		})
	}
}

func auditFailures(repo AuditLogger, status int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := newAuditResponseWriter(w)
			next.ServeHTTP(wrapAuditResponseWriter(rw), r)
			if rw.status != status {
				return
			}

			event := auditEventFor(r.Method, r.URL.Path)
			entry := audit.Entry{
				Action:    event.action,
				Resource:  event.resource,
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  auditMetadata(r, rw.status),
			}

			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			if event.critical {
				defer cancel()
				if err := repo.Log(ctx, entry); err != nil {
					logAuditFailure(err, entry)
				}
				return
			}
			go func() {
				defer cancel()
				if err := repo.Log(ctx, entry); err != nil {
					logAuditFailure(err, entry)
				}
			}()
		})
	}
}

func newAuditResponseWriter(w http.ResponseWriter) *auditResponseWriter {
	return &auditResponseWriter{ResponseWriter: w, status: http.StatusOK}
}

func wrapAuditResponseWriter(w *auditResponseWriter) http.ResponseWriter {
	_, hasFlusher := w.ResponseWriter.(http.Flusher)
	_, hasHijacker := w.ResponseWriter.(http.Hijacker)
	_, hasPusher := w.ResponseWriter.(http.Pusher)

	switch {
	case hasFlusher && hasHijacker && hasPusher:
		return auditFlusherHijackerPusher{w}
	case hasFlusher && hasHijacker:
		return auditFlusherHijacker{w}
	case hasFlusher && hasPusher:
		return auditFlusherPusher{w}
	case hasHijacker && hasPusher:
		return auditHijackerPusher{w}
	case hasFlusher:
		return auditFlusher{w}
	case hasHijacker:
		return auditHijacker{w}
	case hasPusher:
		return auditPusher{w}
	default:
		return w
	}
}

type auditEvent struct {
	action   string
	resource string
	critical bool
}

func auditEventFor(method, path string) auditEvent {
	event := auditEvent{
		action:   method + " " + path,
		resource: path,
	}
	if method == http.MethodPost && path == "/api/v1/auth/login" {
		event.action = "request.auth.login"
		return event
	}
	if method == http.MethodPost && path == "/api/v1/auth/mfa/challenge" {
		event.action = "request.auth.mfa_challenge"
		return event
	}
	if method == http.MethodPost && path == "/api/v1/auth/token" {
		event.action = "request.auth.token"
		return event
	}
	if method == http.MethodPost && path == "/api/v1/auth/refresh" {
		event.action = "request.auth.refresh"
		return event
	}
	if method == http.MethodPost && path == "/api/v1/auth/logout" {
		event.action = "request.auth.logout"
		return event
	}
	if method == http.MethodPost && path == "/api/v1/auth/register" {
		event.action = "request.auth.register"
		return event
	}
	if method == http.MethodPost && path == "/api/v1/auth/forgot-password" {
		event.action = "request.auth.password_reset_request"
		return event
	}
	if method == http.MethodPost && path == "/api/v1/auth/reset-password" {
		event.action = "request.auth.password_reset"
		return event
	}
	if method == http.MethodGet && path == "/api/v1/admin/service-accounts/events" {
		event.action = "request.service_account.events"
		return event
	}
	if method == http.MethodPatch && strings.HasPrefix(path, "/api/v1/admin/users/") && strings.HasSuffix(path, "/roles") {
		event.action = "admin.user.roles_update"
		event.critical = true
		return event
	}
	if method == http.MethodPost && path == "/api/v1/admin/users" {
		event.action = "admin.user.create"
		event.critical = true
		return event
	}
	if method == http.MethodPatch && strings.HasPrefix(path, "/api/v1/admin/users/") && strings.HasSuffix(path, "/status") {
		event.action = "admin.user.status_update"
		event.critical = true
		return event
	}
	if method == http.MethodDelete && path == "/api/v1/sessions" {
		event.action = "session.revoke_others"
		event.critical = true
		return event
	}
	if method == http.MethodDelete && strings.HasPrefix(path, "/api/v1/sessions/") {
		event.action = "session.revoke"
		event.critical = true
		return event
	}
	if method == http.MethodDelete && strings.HasPrefix(path, "/api/v1/admin/users/") && strings.Contains(path, "/sessions") {
		if strings.Count(path, "/") > strings.Count("/api/v1/admin/users/u/sessions", "/") {
			event.action = "admin.user.session_revoke"
		} else {
			event.action = "admin.user.sessions_revoke_all"
		}
		event.critical = true
		return event
	}
	if method == http.MethodPatch && path == "/api/v1/admin/settings" {
		event.action = "admin.settings.update"
		event.critical = true
		return event
	}
	if method == http.MethodPost && path == "/api/v1/admin/service-accounts" {
		event.action = "service_account.create"
		event.critical = true
		return event
	}
	if strings.HasPrefix(path, "/api/v1/admin/service-accounts/") {
		switch {
		case method == http.MethodPatch && strings.HasSuffix(path, "/status"):
			event.action = "service_account.status_update"
			event.critical = true
			return event
		case method == http.MethodPatch:
			event.action = "service_account.update"
			event.critical = true
			return event
		case method == http.MethodDelete:
			event.action = "service_account.delete"
			event.critical = true
			return event
		}
	}
	if method == http.MethodPost && path == "/api/v1/mfa/setup" {
		event.action = "mfa.setup_start"
		event.critical = true
		return event
	}
	if method == http.MethodPost && path == "/api/v1/mfa/verify" {
		event.action = "mfa.verify"
		event.critical = true
		return event
	}
	if method == http.MethodPost && path == "/api/v1/mfa/disable" {
		event.action = "mfa.disable"
		event.critical = true
		return event
	}
	if method == http.MethodPost && path == "/api/v1/mfa/step-up" {
		event.action = "mfa.step_up"
		event.critical = true
		return event
	}
	if method == http.MethodPost && path == "/api/v1/webauthn/register/begin" {
		event.action = "webauthn.register_start"
		return event
	}
	if method == http.MethodPost && path == "/api/v1/webauthn/register/finish" {
		event.action = "webauthn.register_finish"
		event.critical = true
		return event
	}
	if method == http.MethodDelete && strings.HasPrefix(path, "/api/v1/webauthn/credentials/") {
		event.action = "webauthn.credential_delete"
		event.critical = true
		return event
	}
	return event
}

func logAuditFailure(err error, entry audit.Entry) {
	audit.RecordWriteFailure()
	attrs := []any{
		"error", err,
		"action", entry.Action,
		"resource", entry.Resource,
	}
	if entry.UserID != nil {
		attrs = append(attrs, "user_id", *entry.UserID)
	}
	slog.Error("audit log write failed", attrs...)
}

func auditMetadata(r *http.Request, status int) map[string]any {
	metadata := map[string]any{
		"status":  status,
		"outcome": auditOutcome(status),
	}

	raw := r.Header.Get("X-Client-Info")
	if raw == "" {
		return metadata
	}

	var clientInfo map[string]string
	if err := json.Unmarshal([]byte(raw), &clientInfo); err != nil || len(clientInfo) == 0 {
		return metadata
	}

	clean := make(map[string]string, len(clientInfo))
	for key, value := range clientInfo {
		if key == "" || value == "" || len(key) > 40 || len(value) > 100 {
			continue
		}
		clean[key] = value
	}
	if len(clean) > 0 {
		metadata["client_info"] = clean
	}

	return metadata
}

func auditOutcome(status int) string {
	if status >= 200 && status < 400 {
		return "success"
	}
	return "failure"
}

type auditResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *auditResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *auditResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *auditResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *auditResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(r)
	}
	return io.Copy(w.ResponseWriter, r)
}

type auditFlusher struct {
	*auditResponseWriter
}

func (w auditFlusher) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
}

type auditHijacker struct {
	*auditResponseWriter
}

func (w auditHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

type auditPusher struct {
	*auditResponseWriter
}

func (w auditPusher) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

type auditFlusherHijacker struct {
	*auditResponseWriter
}

func (w auditFlusherHijacker) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
}

func (w auditFlusherHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

type auditFlusherPusher struct {
	*auditResponseWriter
}

func (w auditFlusherPusher) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
}

func (w auditFlusherPusher) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}

type auditHijackerPusher struct {
	*auditResponseWriter
}

func (w auditHijackerPusher) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

func (w auditHijackerPusher) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}

type auditFlusherHijackerPusher struct {
	*auditResponseWriter
}

func (w auditFlusherHijackerPusher) Flush() {
	w.ResponseWriter.(http.Flusher).Flush()
}

func (w auditFlusherHijackerPusher) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.ResponseWriter.(http.Hijacker).Hijack()
}

func (w auditFlusherHijackerPusher) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}

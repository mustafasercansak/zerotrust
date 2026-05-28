package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zerotrust/backend/internal/audit"
)

type recordingAuthAuditLogger struct {
	entries []audit.Entry
	err     error
	done    chan struct{}
}

func (l *recordingAuthAuditLogger) Log(ctx context.Context, entry audit.Entry) error {
	l.entries = append(l.entries, entry)
	l.done <- struct{}{}
	return l.err
}

func (l *recordingAuthAuditLogger) wait(t *testing.T) audit.Entry {
	t.Helper()
	select {
	case <-l.done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for audit write")
	}
	if len(l.entries) == 0 {
		t.Fatal("missing audit entry")
	}
	return l.entries[len(l.entries)-1]
}

type fakeAuthService struct {
	loginResult       *LoginResult
	loginErr          error
	clientCredentials *ServiceTokenResponse
	clientErr         error
	mfaPair           *TokenPair
	mfaErr            error
	refreshPair       *TokenPair
	refreshErr        error
}

func (s *fakeAuthService) ClientCredentials(ctx context.Context, clientID, secret string) (*ServiceTokenResponse, error) {
	if s.clientErr != nil {
		return nil, s.clientErr
	}
	return s.clientCredentials, nil
}

func (s *fakeAuthService) Login(ctx context.Context, email, password, ip, ua string, deviceInfo map[string]string) (*LoginResult, error) {
	return s.loginResult, s.loginErr
}

func (s *fakeAuthService) MFAChallenge(ctx context.Context, pendingToken, totpCode string) (*TokenPair, error) {
	if s.mfaErr != nil {
		return nil, s.mfaErr
	}
	return s.mfaPair, nil
}

func (s *fakeAuthService) RefreshTokens(ctx context.Context, refreshToken, ip, ua string, deviceInfo map[string]string) (*TokenPair, error) {
	if s.refreshErr != nil {
		return nil, s.refreshErr
	}
	return s.refreshPair, nil
}

func (s *fakeAuthService) Logout(ctx context.Context, refreshToken, accessToken string) error {
	return nil
}

type fakePasswordResetter struct {
	resetErr error
}

func (p *fakePasswordResetter) SendReset(ctx context.Context, email, baseURL string) error {
	return nil
}

func (p *fakePasswordResetter) Reset(ctx context.Context, token, newPassword string) error {
	return p.resetErr
}

func TestLoginSuccessWritesAuditEvent(t *testing.T) {
	logger := &recordingAuthAuditLogger{done: make(chan struct{}, 1)}
	svc := &fakeAuthService{loginResult: &LoginResult{
		Pair: &TokenPair{AccessToken: "access-token", RefreshToken: "refresh-token"},
	}}
	h := NewHandler(svc, nil, logger, false, false, nil, "")

	body, _ := json.Marshal(map[string]any{
		"email":    "admin@example.com",
		"password": "correct-password",
		"client_info": map[string]string{
			"os": "Linux",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("User-Agent", "test-agent")
	rr := httptest.NewRecorder()

	h.Login(rr, req)
	entry := logger.wait(t)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if entry.Action != "auth.login_success" {
		t.Fatalf("Action=%q want auth.login_success", entry.Action)
	}
	if entry.Resource != "/api/v1/auth/login" {
		t.Fatalf("Resource=%q want /api/v1/auth/login", entry.Resource)
	}
	if entry.IPAddress != "203.0.113.10:1234" || entry.UserAgent != "test-agent" {
		t.Fatalf("unexpected client context ip=%q ua=%q", entry.IPAddress, entry.UserAgent)
	}
	if entry.Metadata["email"] != "admin@example.com" {
		t.Fatalf("metadata email=%v want admin@example.com", entry.Metadata["email"])
	}
	if entry.Metadata["status"] != http.StatusOK {
		t.Fatalf("metadata status=%v want %d", entry.Metadata["status"], http.StatusOK)
	}
	if entry.Metadata["outcome"] != "success" {
		t.Fatalf("metadata outcome=%v want success", entry.Metadata["outcome"])
	}
}

func TestLoginFailureWritesAuditEvent(t *testing.T) {
	logger := &recordingAuthAuditLogger{done: make(chan struct{}, 1)}
	h := NewHandler(&fakeAuthService{loginErr: ErrInvalidCredentials}, nil, logger, false, false, nil, "")

	body, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "wrong-password"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.Login(rr, req)
	entry := logger.wait(t)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
	if entry.Action != "auth.login_failed" {
		t.Fatalf("Action=%q want auth.login_failed", entry.Action)
	}
	if entry.Metadata["reason"] != ErrInvalidCredentials.Error() {
		t.Fatalf("metadata reason=%v want %q", entry.Metadata["reason"], ErrInvalidCredentials.Error())
	}
	if entry.Metadata["status"] != http.StatusUnauthorized {
		t.Fatalf("metadata status=%v want %d", entry.Metadata["status"], http.StatusUnauthorized)
	}
	if entry.Metadata["outcome"] != "failure" {
		t.Fatalf("metadata outcome=%v want failure", entry.Metadata["outcome"])
	}
}

func TestLoginLockoutWritesAuditEvent(t *testing.T) {
	logger := &recordingAuthAuditLogger{done: make(chan struct{}, 1)}
	h := NewHandler(&fakeAuthService{loginErr: &AccountLockedError{RetryAfter: time.Minute}}, nil, logger, false, false, nil, "")

	body, _ := json.Marshal(map[string]string{"email": "admin@example.com", "password": "wrong-password"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.Login(rr, req)
	entry := logger.wait(t)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusTooManyRequests, rr.Body.String())
	}
	if entry.Action != "auth.login_failed" {
		t.Fatalf("Action=%q want auth.login_failed", entry.Action)
	}
	if entry.Metadata["reason"] != "account_locked" {
		t.Fatalf("metadata reason=%v want account_locked", entry.Metadata["reason"])
	}
	if entry.Metadata["status"] != http.StatusTooManyRequests {
		t.Fatalf("metadata status=%v want %d", entry.Metadata["status"], http.StatusTooManyRequests)
	}
}

func TestClientCredentialsSuccessWritesAuditEvent(t *testing.T) {
	logger := &recordingAuthAuditLogger{done: make(chan struct{}, 1)}
	h := NewHandler(&fakeAuthService{
		clientCredentials: &ServiceTokenResponse{AccessToken: "service-token", ExpiresIn: 60},
	}, nil, logger, false, false, nil, "")

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     "client-1",
		"client_secret": "secret",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.Token(rr, req)
	entry := logger.wait(t)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if entry.Action != "auth.client_credentials_success" {
		t.Fatalf("Action=%q want auth.client_credentials_success", entry.Action)
	}
	if entry.Metadata["client_id"] != "client-1" {
		t.Fatalf("metadata client_id=%v want client-1", entry.Metadata["client_id"])
	}
	if _, ok := entry.Metadata["client_secret"]; ok {
		t.Fatal("client_secret must never be written to audit metadata")
	}
}

func TestClientCredentialsFailureWritesAuditEvent(t *testing.T) {
	logger := &recordingAuthAuditLogger{done: make(chan struct{}, 1)}
	h := NewHandler(&fakeAuthService{clientErr: errors.New("invalid client")}, nil, logger, false, false, nil, "")

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     "client-1",
		"client_secret": "wrong",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.Token(rr, req)
	entry := logger.wait(t)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
	if entry.Action != "auth.client_credentials_failed" {
		t.Fatalf("Action=%q want auth.client_credentials_failed", entry.Action)
	}
	if entry.Metadata["reason"] != "invalid_client" {
		t.Fatalf("metadata reason=%v want invalid_client", entry.Metadata["reason"])
	}
}

func TestMFAChallengeWritesAuditEvents(t *testing.T) {
	tests := []struct {
		name   string
		svc    *fakeAuthService
		status int
		action string
	}{
		{
			name:   "success",
			svc:    &fakeAuthService{mfaPair: &TokenPair{AccessToken: "access-token", RefreshToken: "refresh-token"}},
			status: http.StatusOK,
			action: "auth.mfa_challenge_success",
		},
		{
			name:   "failure",
			svc:    &fakeAuthService{mfaErr: errors.New("bad code")},
			status: http.StatusUnauthorized,
			action: "auth.mfa_challenge_failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &recordingAuthAuditLogger{done: make(chan struct{}, 1)}
			h := NewHandler(tt.svc, nil, logger, false, false, nil, "")

			body, _ := json.Marshal(map[string]string{"mfa_token": "pending-token", "totp_code": "123456"})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/challenge", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			h.MFAChallenge(rr, req)
			entry := logger.wait(t)

			if rr.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tt.status, rr.Body.String())
			}
			if entry.Action != tt.action {
				t.Fatalf("Action=%q want %q", entry.Action, tt.action)
			}
			if entry.Metadata["status"] != tt.status {
				t.Fatalf("metadata status=%v want %d", entry.Metadata["status"], tt.status)
			}
			if _, ok := entry.Metadata["totp_code"]; ok {
				t.Fatal("totp_code must never be written to audit metadata")
			}
			if _, ok := entry.Metadata["mfa_token"]; ok {
				t.Fatal("mfa_token must never be written to audit metadata")
			}
		})
	}
}

func TestRefreshFailureWritesAuditEvent(t *testing.T) {
	logger := &recordingAuthAuditLogger{done: make(chan struct{}, 1)}
	h := NewHandler(&fakeAuthService{refreshErr: errors.New("bad refresh")}, nil, logger, false, false, nil, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{}`))
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: "refresh-token"})
	rr := httptest.NewRecorder()

	h.Refresh(rr, req)
	entry := logger.wait(t)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
	if entry.Action != "auth.refresh_failed" {
		t.Fatalf("Action=%q want auth.refresh_failed", entry.Action)
	}
	if entry.Metadata["reason"] != "invalid_token" {
		t.Fatalf("metadata reason=%v want invalid_token", entry.Metadata["reason"])
	}
	if _, ok := entry.Metadata["refresh_token"]; ok {
		t.Fatal("refresh_token must never be written to audit metadata")
	}
}

func TestForgotPasswordWritesAuditEvent(t *testing.T) {
	logger := &recordingAuthAuditLogger{done: make(chan struct{}, 1)}
	h := NewHandler(&fakeAuthService{}, nil, logger, false, false, &fakePasswordResetter{}, "")

	body, _ := json.Marshal(map[string]string{"email": "user@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.ForgotPassword(rr, req)
	entry := logger.wait(t)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if entry.Action != "auth.password_reset_requested" {
		t.Fatalf("Action=%q want auth.password_reset_requested", entry.Action)
	}
	if entry.Metadata["email"] != "user@example.com" {
		t.Fatalf("metadata email=%v want user@example.com", entry.Metadata["email"])
	}
}

func TestResetPasswordWritesAuditEvents(t *testing.T) {
	tests := []struct {
		name     string
		resetErr error
		status   int
		action   string
	}{
		{name: "success", status: http.StatusOK, action: "auth.password_reset_success"},
		{name: "failure", resetErr: errors.New("bad token"), status: http.StatusBadRequest, action: "auth.password_reset_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &recordingAuthAuditLogger{done: make(chan struct{}, 1)}
			h := NewHandler(&fakeAuthService{}, nil, logger, false, false, &fakePasswordResetter{resetErr: tt.resetErr}, "")

			body, _ := json.Marshal(map[string]string{"token": "reset-token", "password": "StrongerPass123!"})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", bytes.NewReader(body))
			rr := httptest.NewRecorder()

			h.ResetPassword(rr, req)
			entry := logger.wait(t)

			if rr.Code != tt.status {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tt.status, rr.Body.String())
			}
			if entry.Action != tt.action {
				t.Fatalf("Action=%q want %q", entry.Action, tt.action)
			}
			if _, ok := entry.Metadata["token"]; ok {
				t.Fatal("reset token must never be written to audit metadata")
			}
			if _, ok := entry.Metadata["password"]; ok {
				t.Fatal("password must never be written to audit metadata")
			}
		})
	}
}

func TestLogoutWritesAuditEvent(t *testing.T) {
	logger := &recordingAuthAuditLogger{done: make(chan struct{}, 1)}
	h := NewHandler(nil, nil, logger, false, false, nil, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("User-Agent", "test-agent")
	rr := httptest.NewRecorder()

	h.Logout(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNoContent, rr.Body.String())
	}

	entry := logger.wait(t)

	if entry.Action != "auth.logout" {
		t.Fatalf("Action=%q want auth.logout", entry.Action)
	}
	if entry.Resource != "/api/v1/auth/logout" {
		t.Fatalf("Resource=%q want /api/v1/auth/logout", entry.Resource)
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
}

func TestAuthAuditWriteFailureIsLogged(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	logger := &recordingAuthAuditLogger{err: errors.New("db down"), done: make(chan struct{}, 1)}
	h := NewHandler(&fakeAuthService{}, nil, logger, false, false, nil, "")
	failuresBefore := audit.WriteFailures()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rr := httptest.NewRecorder()

	h.Logout(rr, req)
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
	if !strings.Contains(logged, "auth.logout") {
		t.Fatalf("expected action in log, got %q", logged)
	}
	if got := audit.WriteFailures(); got != failuresBefore+1 {
		t.Fatalf("audit write failures=%d want %d", got, failuresBefore+1)
	}
}

package mailer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type mockBaseMailer struct {
	mu           sync.Mutex
	alertCount   int
	alertFail    bool
	alertFailMax int
	resetCount   int
	resetFail    bool
	resetFailMax int
}

func (m *mockBaseMailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetCount++
	if m.resetFail {
		if m.resetFailMax == 0 || m.resetCount <= m.resetFailMax {
			return errors.New("smtp connection refused")
		}
	}
	return nil
}

func (m *mockBaseMailer) SendSecurityAlert(ctx context.Context, to, alertType, ipAddress, location, details string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alertCount++
	if m.alertFail {
		if m.alertFailMax == 0 || m.alertCount <= m.alertFailMax {
			return errors.New("smtp connection refused")
		}
	}
	return nil
}

func TestResilientMailer_SuccessFirstTry(t *testing.T) {
	base := &mockBaseMailer{}
	rm := NewResilientMailer(base, 10, nil)
	rm.BaseDelay = 1 * time.Millisecond
	rm.Start(1)
	defer rm.Stop()

	err := rm.SendSecurityAlert(context.Background(), "user@example.com", "impossible_travel", "1.1.1.1", "US", "details")
	if err != nil {
		t.Fatalf("unexpected queue error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	base.mu.Lock()
	defer base.mu.Unlock()
	if base.alertCount != 1 {
		t.Errorf("expected 1 send attempt, got %d", base.alertCount)
	}
}

func TestResilientMailer_RetryAndSuccess(t *testing.T) {
	base := &mockBaseMailer{
		alertFail:    true,
		alertFailMax: 2, // fail first two, third succeeds
	}

	rm := NewResilientMailer(base, 10, nil)
	rm.BaseDelay = 1 * time.Millisecond
	rm.Start(1)
	defer rm.Stop()

	err := rm.SendSecurityAlert(context.Background(), "user@example.com", "impossible_travel", "1.1.1.1", "US", "details")
	if err != nil {
		t.Fatalf("unexpected queue error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	base.mu.Lock()
	defer base.mu.Unlock()
	if base.alertCount != 3 {
		t.Errorf("expected 3 attempts (2 fails, 1 success), got %d", base.alertCount)
	}
}

func TestResilientMailer_MaxRetriesReached(t *testing.T) {
	base := &mockBaseMailer{
		alertFail:    true,
		alertFailMax: 10, // fail everything
	}

	var mu sync.Mutex
	var auditedEmail, auditedType, auditedIp string
	var auditedErr error

	auditFn := func(ctx context.Context, email, alertType, ip, details string, err error) {
		mu.Lock()
		defer mu.Unlock()
		auditedEmail = email
		auditedType = alertType
		auditedIp = ip
		auditedErr = err
	}

	rm := NewResilientMailer(base, 10, auditFn)
	rm.BaseDelay = 1 * time.Millisecond
	rm.Start(1)
	defer rm.Stop()

	err := rm.SendSecurityAlert(context.Background(), "user@example.com", "impossible_travel", "1.1.1.1", "US", "details")
	if err != nil {
		t.Fatalf("unexpected queue error: %v", err)
	}

	// MaxRetries = 5, backoff is 1ms, 2ms, 4ms, 8ms. Should finish in < 50ms.
	time.Sleep(100 * time.Millisecond)

	base.mu.Lock()
	if base.alertCount != 5 {
		t.Errorf("expected exactly 5 attempts, got %d", base.alertCount)
	}
	base.mu.Unlock()

	mu.Lock()
	defer mu.Unlock()
	if auditedEmail != "user@example.com" {
		t.Errorf("expected callback for user@example.com, got %q", auditedEmail)
	}
	if auditedType != "impossible_travel" {
		t.Errorf("expected callback type impossible_travel, got %q", auditedType)
	}
	if auditedIp != "1.1.1.1" {
		t.Errorf("expected callback IP 1.1.1.1, got %q", auditedIp)
	}
	if auditedErr == nil {
		t.Errorf("expected callback error, got nil")
	}
}

// TestResilientMailer_SendPasswordResetDelegatesToUnderlying proves password
// resets are queued on the same bounded worker pool as security alerts
// (ISSUE_LIST #93), rather than firing an unbounded goroutine per request,
// and that the worker eventually delivers via the underlying mailer.
func TestResilientMailer_SendPasswordResetDelegatesToUnderlying(t *testing.T) {
	base := &mockBaseMailer{}
	rm := NewResilientMailer(base, 10, nil)
	rm.BaseDelay = 1 * time.Millisecond
	rm.Start(1)
	defer rm.Stop()

	if err := rm.SendPasswordReset(context.Background(), "user@example.com", "https://example/reset"); err != nil {
		t.Fatalf("SendPasswordReset returned error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	base.mu.Lock()
	defer base.mu.Unlock()
	if base.resetCount != 1 {
		t.Fatalf("expected 1 password reset send, got %d", base.resetCount)
	}
}

// TestResilientMailer_PasswordResetQueueFull proves a full mail queue rejects
// a password-reset request instead of blocking or spawning an unbounded
// goroutine. (ISSUE_LIST #93)
func TestResilientMailer_PasswordResetQueueFull(t *testing.T) {
	base := &mockBaseMailer{}
	rm := NewResilientMailer(base, 1, nil)
	rm.BaseDelay = time.Millisecond

	rm.jobs <- AlertJob{To: "existing@example.com"}

	err := rm.SendPasswordReset(context.Background(), "user@example.com", "https://example/reset")
	if err == nil {
		t.Fatal("expected queue full error, got nil")
	}
}

// TestResilientMailer_PasswordResetRetriesOnFailure proves password-reset
// jobs use the same retry/backoff path as security alerts.
func TestResilientMailer_PasswordResetRetriesOnFailure(t *testing.T) {
	base := &mockBaseMailer{resetFail: true, resetFailMax: 2}
	rm := NewResilientMailer(base, 10, nil)
	rm.BaseDelay = 1 * time.Millisecond
	rm.Start(1)
	defer rm.Stop()

	if err := rm.SendPasswordReset(context.Background(), "user@example.com", "https://example/reset"); err != nil {
		t.Fatalf("SendPasswordReset returned error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	base.mu.Lock()
	defer base.mu.Unlock()
	if base.resetCount != 3 {
		t.Errorf("expected 3 attempts (2 fails, 1 success), got %d", base.resetCount)
	}
}

func TestResilientMailer_SendSecurityAlertReturnsErrorWhenQueueIsFull(t *testing.T) {
	base := &mockBaseMailer{}
	rm := NewResilientMailer(base, 1, nil)
	rm.BaseDelay = time.Millisecond

	rm.jobs <- AlertJob{To: "existing@example.com"}

	err := rm.SendSecurityAlert(context.Background(), "user@example.com", "impossible_travel", "1.1.1.1", "US", "details")
	if err == nil {
		t.Fatal("expected queue full error, got nil")
	}
}

func TestResilientMailer_EdgeCases(t *testing.T) {
	base := &mockBaseMailer{alertFail: true}

	// 1. BaseDelay is 0
	rm := NewResilientMailer(base, 1, nil)
	rm.BaseDelay = 0
	err := rm.SendSecurityAlert(context.Background(), "user@example.com", "impossible_travel", "1.1.1.1", "US", "details")
	if err != nil {
		t.Fatalf("unexpected queue error: %v", err)
	}

	// 2. MaxRetriesReached with nil auditLogFn
	rm2 := NewResilientMailer(base, 1, nil)
	rm2.BaseDelay = time.Millisecond
	rm2.processJob(AlertJob{
		To:         "user@example.com",
		MaxRetries: 1,
		Attempt:    1,
	})

	// 3. Retry fallback when queue is full
	rm3 := NewResilientMailer(base, 1, nil)
	rm3.BaseDelay = time.Millisecond
	// Put a job in the queue to block retry
	rm3.jobs <- AlertJob{To: "blocker@example.com"}

	rm3.processJob(AlertJob{
		To:         "user@example.com",
		MaxRetries: 5,
		Attempt:    1,
		BaseDelay:  time.Millisecond,
	})

	// Clean up / stop rm3 context to exit any spawned fallback goroutines
	rm3.Stop()
}

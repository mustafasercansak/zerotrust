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
}

func (m *mockBaseMailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetCount++
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

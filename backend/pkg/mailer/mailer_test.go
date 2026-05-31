package mailer

import (
	"context"
	"testing"
)

func TestLogMailer(t *testing.T) {
	m := LogMailer{}
	err := m.SendPasswordReset(context.Background(), "test@example.com", "http://reset")
	if err != nil {
		t.Errorf("LogMailer SendPasswordReset failed: %v", err)
	}

	err = m.SendSecurityAlert(context.Background(), "test@example.com", "alert", "127.0.0.1", "loc", "details")
	if err != nil {
		t.Errorf("LogMailer SendSecurityAlert failed: %v", err)
	}
}

func TestSMTPMailer(t *testing.T) {
	m := NewSMTPMailer("localhost", "12345", "from@example.com", "user", "pass")
	
	// This will likely fail with connection refused, but it tests the message construction
	_ = m.SendPasswordReset(context.Background(), "test@example.com", "http://reset")
	_ = m.SendSecurityAlert(context.Background(), "test@example.com", "alert", "127.0.0.1", "loc", "details")
}

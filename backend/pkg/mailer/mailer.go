package mailer

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// Mailer sends transactional emails.
type Mailer interface {
	SendPasswordReset(ctx context.Context, to, resetURL string) error
	SendSecurityAlert(ctx context.Context, to, alertType, ipAddress, location, details string) error
}

// LogMailer logs the reset URL to stdout instead of sending email.
// Use in development when SMTP is not configured.
type LogMailer struct{}

func (LogMailer) SendPasswordReset(_ context.Context, to, resetURL string) error {
	slog.Info("password reset link (dev — not sent)", "to", to, "url", resetURL)
	return nil
}

func (LogMailer) SendSecurityAlert(_ context.Context, to, alertType, ipAddress, location, details string) error {
	slog.Info("security alert (dev — not sent)", "to", to, "type", alertType, "ip", ipAddress, "location", location, "details", details)
	return nil
}

// SMTPMailer sends emails via SMTP.
type SMTPMailer struct {
	host string
	port string
	from string
	auth smtp.Auth
}

func NewSMTPMailer(host, port, from, user, password string) *SMTPMailer {
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, password, host)
	}
	return &SMTPMailer{host: host, port: port, from: from, auth: auth}
}

func (m *SMTPMailer) SendPasswordReset(_ context.Context, to, resetURL string) error {
	subject := "Reset your ZeroTrust password"
	body := fmt.Sprintf("Click the link below to reset your password. It expires in 1 hour.\n\n%s\n\nIf you did not request a reset, ignore this email.", resetURL)

	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	addr := m.host + ":" + m.port
	return smtp.SendMail(addr, m.auth, m.from, []string{to}, []byte(msg))
}

func (m *SMTPMailer) SendSecurityAlert(_ context.Context, to, alertType, ipAddress, location, details string) error {
	subject := fmt.Sprintf("Security Alert: Unusual login activity detected")
	body := fmt.Sprintf("Hello,\n\nWe detected unusual login activity on your ZeroTrust account:\n\n"+
		"Alert Type: %s\n"+
		"IP Address: %s\n"+
		"Location:   %s\n"+
		"Details:    %s\n\n"+
		"If this was not you, please log in and revoke this session immediately from your settings page.",
		alertType, ipAddress, location, details)

	msg := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	addr := m.host + ":" + m.port
	return smtp.SendMail(addr, m.auth, m.from, []string{to}, []byte(msg))
}

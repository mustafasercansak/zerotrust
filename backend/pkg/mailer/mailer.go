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

func alertSubject(alertType string) string {
	subjects := map[string]string{
		"new_login":          "New sign-in to your account",
		"impossible_travel":  "Security Alert: Login from unexpected location",
		"new_ip":             "Security Alert: Login from new IP address",
		"new_ua":             "Security Alert: Login from new device",
		"account_lockout":    "Security Alert: Your account has been locked",
		"password_changed":   "Security Alert: Your password was changed",
		"locale_changed":     "Security Alert: Your account language was changed",
		"mfa_enabled":        "Security Alert: Two-factor authentication enabled",
		"mfa_disabled":       "Security Alert: Two-factor authentication disabled",
		"passkey_registered": "Security Alert: A new passkey was added",
		"passkey_removed":    "Security Alert: A passkey was removed",
	}
	if s, ok := subjects[alertType]; ok {
		return "ZeroTrust — " + s
	}
	return "ZeroTrust — Security Alert"
}

func (m *SMTPMailer) SendSecurityAlert(_ context.Context, to, alertType, ipAddress, location, details string) error {
	subject := alertSubject(alertType)
	body := fmt.Sprintf("Hello,\n\nSecurity notice for your ZeroTrust account:\n\n"+
		"Event:      %s\n"+
		"IP Address: %s\n"+
		"Location:   %s\n"+
		"Details:    %s\n\n"+
		"If this was not you, please log in and revoke all sessions immediately from your settings page.",
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

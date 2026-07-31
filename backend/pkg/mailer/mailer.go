package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Mailer sends transactional emails.
type Mailer interface {
	SendPasswordReset(ctx context.Context, to, resetURL string) error
	SendSecurityAlert(ctx context.Context, to, alertType, ipAddress, location, details string) error
}

// LogMailer logs mail metadata to stdout instead of sending email.
// Use in development when SMTP is not configured. It must never log message
// bodies or URLs: password-reset links carry live account-takeover tokens and
// logs are broadly accessible (docker logs, journald, Loki) (#90).
type LogMailer struct{}

func (LogMailer) SendPasswordReset(_ context.Context, to, _ string) error {
	slog.Info("password reset requested (dev — not sent)", "to", to)
	return nil
}

func (LogMailer) SendSecurityAlert(_ context.Context, to, alertType, ipAddress, location, details string) error {
	slog.Info("security alert (dev — not sent)", "to", to, "type", alertType, "ip", ipAddress, "location", location, "details", details)
	return nil
}

// ErrPlaintextSMTP is returned when the SMTP server offers no STARTTLS and
// plaintext delivery has not been explicitly allowed (dev mode only).
var ErrPlaintextSMTP = errors.New("smtp server does not support STARTTLS; refusing plaintext delivery outside dev mode")

// SMTPMailer sends emails via SMTP. Delivery fails closed unless the
// connection is TLS: implicit TLS when the port is 465, otherwise a verified
// STARTTLS upgrade. Plaintext delivery is only possible when AllowPlaintext
// is set, which the server wires exclusively to DEV_MODE (#89).
type SMTPMailer struct {
	host        string
	port        string
	from        string
	auth        smtp.Auth
	implicitTLS bool
	// AllowPlaintext permits delivery to servers without STARTTLS. Dev mode only.
	AllowPlaintext bool
}

func NewSMTPMailer(host, port, from, user, password string) *SMTPMailer {
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, password, host)
	}
	return &SMTPMailer{host: host, port: port, from: from, auth: auth, implicitTLS: port == "465"}
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

	return m.send(to, msg)
}

// send delivers a prepared message, requiring TLS on the wire: implicit TLS
// when the port is 465, otherwise a verified STARTTLS upgrade. Plaintext
// delivery is refused unless AllowPlaintext is set (dev mode only) (#89).
func (m *SMTPMailer) send(to, msg string) error {
	addr := net.JoinHostPort(m.host, m.port)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return err
	}
	if m.implicitTLS {
		conn = tls.Client(conn, &tls.Config{ServerName: m.host, MinVersion: tls.VersionTLS12})
	}

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		conn.Close() //nolint:errcheck
		return err
	}
	defer client.Close() //nolint:errcheck

	if !m.implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: m.host, MinVersion: tls.VersionTLS12}); err != nil {
				return fmt.Errorf("STARTTLS upgrade failed: %w", err)
			}
		} else if !m.AllowPlaintext {
			return ErrPlaintextSMTP
		}
	}

	if m.auth != nil {
		if err := client.Auth(m.auth); err != nil {
			return err
		}
	}
	if err := client.Mail(m.from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		w.Close() //nolint:errcheck
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
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

	return m.send(to, msg)
}

package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SlackAttachment represents a Slack webhook attachment.
type SlackAttachment struct {
	Fallback string       `json:"fallback"`
	Color    string       `json:"color"`
	Title    string       `json:"title"`
	Fields   []SlackField `json:"fields"`
}

// SlackField represents a key-value field inside a Slack attachment.
type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// SlackPayload represents the Slack webhook message body.
type SlackPayload struct {
	Text        string            `json:"text"`
	Attachments []SlackAttachment `json:"attachments"`
}

// isHighRiskEvent determines if an audit entry represents a high-risk security event.
func isHighRiskEvent(e Entry) bool {
	outcome := ""
	if e.Metadata != nil {
		if o, ok := e.Metadata["outcome"].(string); ok {
			outcome = o
		}
	}

	// 1. All login/MFA authentication failures are high-risk.
	if outcome == "failure" {
		if e.Action == "request.auth.login" || e.Action == "mfa.verify" || strings.HasPrefix(e.Action, "auth.") || strings.HasPrefix(e.Action, "webauthn.") {
			return true
		}
	}

	// 2. Critical administrative/security policy changes or status toggles.
	criticalActions := map[string]bool{
		"admin.user.status_update":       true,
		"admin.user.roles_update":        true,
		"admin.settings.update":          true,
		"mfa.disable":                    true,
		"session.revoke":                 true,
		"session.revoke_others":          true,
		"admin.user.session_revoke":      true,
		"admin.user.sessions_revoke_all": true,
		"webauthn.credential_delete":     true,
		"oidc.client_created":            true,
		"oidc.client_updated":            true,
		"oidc.client_deleted":            true,
	}

	if criticalActions[e.Action] {
		return true
	}

	return false
}

// TestWebhook sends a synthetic test payload to url to verify delivery.
func (r *Repository) TestWebhook(ctx context.Context, url string) error {
	return r.sendWebhook(ctx, url, Entry{
		Action:    "system.webhook_test",
		Resource:  "settings",
		IPAddress: "0.0.0.0",
		Metadata:  map[string]any{"outcome": "success", "note": "This is a test alert from ZeroTrust."},
	})
}

// validateWebhookURL rejects URLs that could be used for Server-Side Request
// Forgery (SSRF). It requires an https scheme — webhook payloads contain user
// email, IP and security-event details that must not travel in cleartext (#104)
// — and ensures the target hostname resolves only to public, non-loopback,
// non-private addresses. Plain http is accepted only when allowInsecure is set
// (the webhook_allow_insecure setting, intended for local development).
func validateWebhookURL(rawURL string, allowInsecure bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "https" && !(allowInsecure && u.Scheme == "http") {
		if allowInsecure {
			return errors.New("webhook URL scheme must be http or https")
		}
		return errors.New("webhook URL scheme must be https (set webhook_allow_insecure for local development)")
	}
	hostname := u.Hostname()
	if hostname == "" {
		return errors.New("webhook URL has no hostname")
	}
	addrs, err := net.LookupHost(hostname)
	if err != nil {
		return fmt.Errorf("cannot resolve webhook hostname %q: %w", hostname, err)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("webhook URL resolves to a private or internal address (%s)", addr)
		}
	}
	return nil
}

// ssrfSafeTransport returns an http.Transport whose DialContext validates every
// resolved IP address at connection time. This closes the DNS-rebinding window
// that exists when validateWebhookURL and http.Client.Do resolve the hostname
// independently: an attacker-controlled DNS server could return a public IP
// during validation, then switch to a private IP for the real TCP dial.
func ssrfSafeTransport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			addrs, err := net.DefaultResolver.LookupHost(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("cannot resolve webhook host %q: %w", host, err)
			}
			for _, a := range addrs {
				ip := net.ParseIP(a)
				if ip == nil {
					continue
				}
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
					return nil, fmt.Errorf("webhook host %q resolves to private/internal address %s", host, a)
				}
			}
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(addrs[0], port))
		},
	}
}

// sendWebhook dispatches the audit log details to a Slack-compatible webhook URL.
func (r *Repository) sendWebhook(ctx context.Context, url string, e Entry) error {
	// Skip SSRF check only when a test client has been explicitly injected.
	if r.webhookClient == nil {
		allowInsecure := r.settings != nil && r.settings.GetBool(ctx, "webhook_allow_insecure", false)
		if err := validateWebhookURL(url, allowInsecure); err != nil {
			return err
		}
	}

	outcome := "success"
	if e.Metadata != nil {
		if o, ok := e.Metadata["outcome"].(string); ok {
			outcome = o
		}
	}

	color := "#f0ad4e" // default: warning (yellow/orange)
	if outcome == "failure" {
		color = "#d9534f" // red (danger)
	} else if strings.Contains(e.Action, "revoke") || strings.Contains(e.Action, "delete") || e.Action == "mfa.disable" {
		color = "#d9534f" // red (danger) for removals/revocations
	}

	title := fmt.Sprintf("Security Event: %s", e.Action)
	fallback := fmt.Sprintf("Security Event: %s (%s)", e.Action, outcome)

	var fields []SlackField
	fields = append(fields, SlackField{Title: "Action", Value: e.Action, Short: true})
	fields = append(fields, SlackField{Title: "Outcome", Value: outcome, Short: true})

	if email, _ := e.Metadata["email"].(string); email != "" {
		fields = append(fields, SlackField{Title: "Email", Value: email, Short: true})
	} else if e.UserID != nil {
		fields = append(fields, SlackField{Title: "User ID", Value: *e.UserID, Short: true})
	}
	if e.IPAddress != "" {
		fields = append(fields, SlackField{Title: "IP Address", Value: e.IPAddress, Short: true})
	}
	if e.Resource != "" {
		fields = append(fields, SlackField{Title: "Resource", Value: e.Resource, Short: false})
	}

	fields = append(fields, SlackField{Title: "Timestamp", Value: time.Now().UTC().Format(time.RFC3339), Short: true})

	payload := SlackPayload{
		Text: "🚨 *ZeroTrust High-Risk Security Alert*",
		Attachments: []SlackAttachment{
			{
				Fallback: fallback,
				Color:    color,
				Title:    title,
				Fields:   fields,
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := r.webhookClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, Transport: ssrfSafeTransport()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("dispatch webhook request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook responded with non-2xx status: %d", resp.StatusCode)
	}

	return nil
}

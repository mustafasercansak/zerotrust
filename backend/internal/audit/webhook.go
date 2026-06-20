package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	}

	if criticalActions[e.Action] {
		return true
	}

	return false
}

// sendWebhook dispatches the audit log details to a Slack-compatible webhook URL.
func (r *Repository) sendWebhook(ctx context.Context, url string, e Entry) error {
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

	client := &http.Client{Timeout: 5 * time.Second}
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

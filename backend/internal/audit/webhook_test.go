package audit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockSettingsReader struct {
	enabled bool
	url     string
}

func (m *mockSettingsReader) GetBool(_ context.Context, key string, defaultVal bool) bool {
	if key == "webhook_enabled" {
		return m.enabled
	}
	return defaultVal
}

func (m *mockSettingsReader) GetString(_ context.Context, key string, defaultVal string) string {
	if key == "webhook_url" {
		return m.url
	}
	return defaultVal
}

func TestIsHighRiskEvent(t *testing.T) {
	tests := []struct {
		name   string
		entry  Entry
		expect bool
	}{
		{
			name:   "login failure",
			entry:  Entry{Action: "request.auth.login", Metadata: map[string]any{"outcome": "failure"}},
			expect: true,
		},
		{
			name:   "login success",
			entry:  Entry{Action: "request.auth.login", Metadata: map[string]any{"outcome": "success"}},
			expect: false,
		},
		{
			name:   "mfa verify failure",
			entry:  Entry{Action: "mfa.verify", Metadata: map[string]any{"outcome": "failure"}},
			expect: true,
		},
		{
			name:   "user status update success",
			entry:  Entry{Action: "admin.user.status_update", Metadata: map[string]any{"outcome": "success"}},
			expect: true,
		},
		{
			name:   "user status update failure",
			entry:  Entry{Action: "admin.user.status_update", Metadata: map[string]any{"outcome": "failure"}},
			expect: true,
		},
		{
			name:   "roles update",
			entry:  Entry{Action: "admin.user.roles_update"},
			expect: true,
		},
		{
			name:   "session revoke",
			entry:  Entry{Action: "session.revoke"},
			expect: true,
		},
		{
			name:   "random low-risk action",
			entry:  Entry{Action: "user.profile_view", Metadata: map[string]any{"outcome": "success"}},
			expect: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isHighRiskEvent(tc.entry)
			if got != tc.expect {
				t.Fatalf("isHighRiskEvent()=%v want=%v", got, tc.expect)
			}
		})
	}
}

func TestSendWebhook(t *testing.T) {
	var receivedPayload SlackPayload
	var mu sync.Mutex
	called := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		called = true

		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}

		if err := json.Unmarshal(bodyBytes, &receivedPayload); err != nil {
			t.Errorf("failed to unmarshal request body to SlackPayload: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := NewRepository(nil)
	ctx := context.Background()
	userID := "user-123"
	entry := Entry{
		UserID:    &userID,
		Action:    "admin.user.status_update",
		Resource:  "/api/v1/admin/users/user-123/status",
		IPAddress: "192.168.1.1",
		Metadata:  map[string]any{"outcome": "success"},
	}

	err := repo.sendWebhook(ctx, server.URL, entry)
	if err != nil {
		t.Fatalf("sendWebhook failed: %v", err)
	}

	mu.Lock()
	if !called {
		t.Fatal("webhook was not called")
	}
	mu.Unlock()

	if !strings.Contains(receivedPayload.Text, "High-Risk Security Alert") {
		t.Errorf("unexpected payload text: %s", receivedPayload.Text)
	}

	if len(receivedPayload.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(receivedPayload.Attachments))
	}

	att := receivedPayload.Attachments[0]
	if att.Color != "#f0ad4e" { // warning color
		t.Errorf("expected attachment color #f0ad4e, got %s", att.Color)
	}

	var foundAction, foundOutcome, foundUser, foundIP bool
	for _, field := range att.Fields {
		switch field.Title {
		case "Action":
			foundAction = true
			if field.Value != "admin.user.status_update" {
				t.Errorf("unexpected Action field value: %s", field.Value)
			}
		case "Outcome":
			foundOutcome = true
			if field.Value != "success" {
				t.Errorf("unexpected Outcome field value: %s", field.Value)
			}
		case "User ID":
			foundUser = true
			if field.Value != "user-123" {
				t.Errorf("unexpected User ID field value: %s", field.Value)
			}
		case "IP Address":
			foundIP = true
			if field.Value != "192.168.1.1" {
				t.Errorf("unexpected IP Address field value: %s", field.Value)
			}
		}
	}

	if !foundAction || !foundOutcome || !foundUser || !foundIP {
		t.Errorf("missing expected fields in slack attachment: %+v", att.Fields)
	}
}

func TestTestWebhook(t *testing.T) {
	var receivedAction string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload SlackPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			for _, f := range payload.Attachments[0].Fields {
				if f.Title == "Action" {
					receivedAction = f.Value
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	repo := NewRepository(nil)
	if err := repo.TestWebhook(context.Background(), server.URL); err != nil {
		t.Fatalf("TestWebhook returned error: %v", err)
	}
	if receivedAction != "system.webhook_test" {
		t.Errorf("expected action system.webhook_test, got %q", receivedAction)
	}
}

func TestTestWebhook_DeliveryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	repo := NewRepository(nil)
	if err := repo.TestWebhook(context.Background(), server.URL); err == nil {
		t.Fatal("expected error on non-2xx response, got nil")
	}
}

func TestRepository_LogTriggersWebhookAsynchronously(t *testing.T) {
	pool, ctx, repo, _ := setupTestDB(t)
	defer pool.Close()

	var mu sync.Mutex
	called := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		called = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	reader := &mockSettingsReader{enabled: true, url: server.URL}
	repo.SetSettingsReader(reader)

	entry := Entry{
		Action:    "admin.user.status_update",
		IPAddress: "127.0.0.1",
		Metadata:  map[string]any{"outcome": "success"},
	}

	if err := repo.Log(ctx, entry); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Since webhook is dispatched in a background goroutine, wait a short moment
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Fatal("expected webhook to be triggered asynchronously")
	}
}

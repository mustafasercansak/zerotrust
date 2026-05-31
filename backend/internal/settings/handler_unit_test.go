package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

type mockSettingsStore struct {
	allResult map[string]string
	allErr    error
	setErr    error
	setCalls  []string
}

func (m *mockSettingsStore) All(_ context.Context) (map[string]string, error) {
	if m.allErr != nil {
		return nil, m.allErr
	}
	return m.allResult, nil
}

func (m *mockSettingsStore) Set(_ context.Context, key, value string) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.setCalls = append(m.setCalls, key+"="+value)
	return nil
}

func TestAllowedKeysValidators(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  bool
	}{
		{name: "max sessions valid", key: "max_sessions_per_user", value: "5", want: true},
		{name: "max sessions invalid", key: "max_sessions_per_user", value: "0", want: false},
		{name: "idle valid", key: "session_idle_timeout_seconds", value: "120", want: true},
		{name: "idle invalid", key: "session_idle_timeout_seconds", value: "10", want: false},
		{name: "admin idle valid", key: "session_idle_timeout_seconds_admin", value: "600", want: true},
		{name: "admin idle invalid", key: "session_idle_timeout_seconds_admin", value: "3601", want: false},
		{name: "absolute valid", key: "session_absolute_timeout_seconds", value: "3600", want: true},
		{name: "absolute invalid", key: "session_absolute_timeout_seconds", value: "120", want: false},
		{name: "complexity valid", key: "password_complexity", value: "strong", want: true},
		{name: "complexity invalid", key: "password_complexity", value: "invalid", want: false},
		{name: "mfa required true", key: "global_mfa_required", value: "true", want: true},
		{name: "mfa required invalid", key: "global_mfa_required", value: "yes", want: false},
		{name: "max login valid", key: "max_login_attempts", value: "3", want: true},
		{name: "max login invalid", key: "max_login_attempts", value: "30", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator, ok := allowedKeys[tt.key]
			if !ok {
				t.Fatalf("missing validator for key %q", tt.key)
			}
			if got := validator(tt.value); got != tt.want {
				t.Fatalf("validator(%q)=%v want=%v", tt.value, got, tt.want)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()

	writeError(rr, http.StatusInternalServerError, "internal_error")

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusInternalServerError)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type=%q want=application/json", ct)
	}

	var payload map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload["error"] != "internal_error" {
		t.Fatalf("error code=%q want=internal_error", payload["error"])
	}
}

func TestHandlerList(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := &mockSettingsStore{allResult: map[string]string{"max_sessions_per_user": "5"}}
		h := NewHandler(store)

		req := httptest.NewRequest("GET", "/api/v1/admin/settings", nil)
		rr := httptest.NewRecorder()
		h.List(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
		}

		var payload map[string]string
		if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if payload["max_sessions_per_user"] != "5" {
			t.Fatalf("payload=%v", payload)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		store := &mockSettingsStore{allErr: errors.New("boom")}
		h := NewHandler(store)

		req := httptest.NewRequest("GET", "/api/v1/admin/settings", nil)
		rr := httptest.NewRecorder()
		h.List(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandlerUpdate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := &mockSettingsStore{}
		h := NewHandler(store)

		body := `{"password_complexity":"strong","global_mfa_required":"true"}`
		req := httptest.NewRequest("PATCH", "/api/v1/admin/settings", bytes.NewBufferString(body))
		rr := httptest.NewRecorder()
		h.Update(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusNoContent)
		}
		sort.Strings(store.setCalls)
		want := []string{"global_mfa_required=true", "password_complexity=strong"}
		for i := range want {
			if i >= len(store.setCalls) || store.setCalls[i] != want[i] {
				t.Fatalf("set calls=%v want=%v", store.setCalls, want)
			}
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		h := NewHandler(&mockSettingsStore{})
		req := httptest.NewRequest("PATCH", "/api/v1/admin/settings", bytes.NewBufferString("{bad"))
		rr := httptest.NewRecorder()
		h.Update(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("unknown setting", func(t *testing.T) {
		h := NewHandler(&mockSettingsStore{})
		req := httptest.NewRequest("PATCH", "/api/v1/admin/settings", bytes.NewBufferString(`{"unknown":"1"}`))
		rr := httptest.NewRecorder()
		h.Update(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("invalid value", func(t *testing.T) {
		h := NewHandler(&mockSettingsStore{})
		req := httptest.NewRequest("PATCH", "/api/v1/admin/settings", bytes.NewBufferString(`{"max_sessions_per_user":"0"}`))
		rr := httptest.NewRecorder()
		h.Update(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("repo set error", func(t *testing.T) {
		h := NewHandler(&mockSettingsStore{setErr: errors.New("boom")})
		req := httptest.NewRequest("PATCH", "/api/v1/admin/settings", bytes.NewBufferString(`{"password_complexity":"low"}`))
		rr := httptest.NewRecorder()
		h.Update(rr, req)
		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusInternalServerError)
		}
	})
}

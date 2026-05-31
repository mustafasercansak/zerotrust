package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

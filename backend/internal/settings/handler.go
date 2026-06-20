package settings

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// allowedKeys defines which settings are admin-editable and validates their values.
var allowedKeys = map[string]func(string) bool{
	"max_sessions_per_user": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= 20
	},
	"session_idle_timeout_seconds": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 60 && n <= 3600
	},
	"session_idle_timeout_seconds_admin": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 60 && n <= 1800
	},
	"session_absolute_timeout_seconds": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1800 && n <= 172800
	},
	"password_complexity": func(v string) bool {
		return v == "low" || v == "medium" || v == "strong"
	},
	"global_mfa_required": func(v string) bool {
		return v == "true" || v == "false"
	},
	"require_hardware_attestation": func(v string) bool {
		return v == "true" || v == "false"
	},
	"max_login_attempts": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= 20
	},
	"webhook_enabled": func(v string) bool {
		return v == "true" || v == "false"
	},
	"webhook_url": func(v string) bool {
		if v == "" {
			return true
		}
		return strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")
	},
}

type Handler struct {
	repo SettingsStore
}

type SettingsStore interface {
	All(ctx context.Context) (map[string]string, error)
	Set(ctx context.Context, key, value string) error
}

func NewHandler(repo SettingsStore) *Handler {
	return &Handler{repo: repo}
}

// GET /api/v1/admin/settings
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	all, err := h.repo.All(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(all)
}

// PATCH /api/v1/admin/settings
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	for k, v := range req {
		validator, ok := allowedKeys[k]
		if !ok {
			writeError(w, http.StatusBadRequest, "unknown_setting")
			return
		}
		if !validator(v) {
			writeError(w, http.StatusBadRequest, "invalid_value")
			return
		}
	}

	for k, v := range req {
		if err := h.repo.Set(r.Context(), k, v); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

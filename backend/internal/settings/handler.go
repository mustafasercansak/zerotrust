package settings

import (
	"context"
	"encoding/json"
	"net"
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
	"country_allowlist": func(v string) bool {
		if v == "" {
			return true
		}
		v = strings.ReplaceAll(v, "\n", ",")
		for _, entry := range strings.Split(v, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			// ISO 3166-1 alpha-2: exactly 2 ASCII letters
			if len(entry) != 2 {
				return false
			}
			for _, c := range entry {
				if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
					return false
				}
			}
		}
		return true
	},
	"ip_allowlist": func(v string) bool {
		if v == "" {
			return true
		}
		v = strings.ReplaceAll(v, "\n", ",")
		for _, entry := range strings.Split(v, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if strings.Contains(entry, "/") {
				if _, _, err := net.ParseCIDR(entry); err != nil {
					return false
				}
			} else {
				if net.ParseIP(entry) == nil {
					return false
				}
			}
		}
		return true
	},
	"device_trust_enabled": func(v string) bool {
		return v == "true" || v == "false"
	},
	"device_trust_allowed_os": func(v string) bool {
		return true
	},
	"device_trust_min_os_version_mac": func(v string) bool {
		return isVersionString(v)
	},
	"device_trust_min_os_version_win": func(v string) bool {
		return isVersionString(v)
	},
	"device_trust_allowed_browsers": func(v string) bool {
		return true
	},
	"device_trust_min_browser_version_chrome": func(v string) bool {
		return isVersionString(v)
	},
	"device_trust_min_browser_version_safari": func(v string) bool {
		return isVersionString(v)
	},
	"device_trust_min_browser_version_firefox": func(v string) bool {
		return isVersionString(v)
	},
	"device_trust_min_browser_version_edge": func(v string) bool {
		return isVersionString(v)
	},
	"device_trust_block_mobile": func(v string) bool {
		return v == "true" || v == "false"
	},
	"risk_based_auth_enabled": func(v string) bool {
		return v == "true" || v == "false"
	},
	"risk_threshold_mfa": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= 100
	},
	"risk_threshold_block": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= 100
	},
	"risk_score_impossible_travel": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 0 && n <= 100
	},
	"risk_score_new_device": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 0 && n <= 100
	},
	"risk_score_suspicious_hours": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 0 && n <= 100
	},
	"risk_score_failed_attempt": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 0 && n <= 50
	},
	"risk_failed_attempt_max_score": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 0 && n <= 100
	},
	"risk_suspicious_hour_start": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 0 && n <= 23
	},
	"risk_suspicious_hour_end": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 0 && n <= 23
	},
	"risk_impossible_travel_velocity_kmh": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 100 && n <= 2000
	},
	"risk_impossible_travel_window_hours": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= 168
	},
	"risk_impossible_travel_min_distance_km": func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= 500
	},
}

type CacheInvalidator interface {
	Invalidate(key string)
}

type Handler struct {
	repo  SettingsStore
	cache CacheInvalidator
}

type SettingsStore interface {
	All(ctx context.Context) (map[string]string, error)
	Set(ctx context.Context, key, value string) error
}

func NewHandler(repo SettingsStore, cache CacheInvalidator) *Handler {
	return &Handler{repo: repo, cache: cache}
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
		if h.cache != nil {
			h.cache.Invalidate(k)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func isVersionString(v string) bool {
	if v == "" {
		return true
	}
	parts := strings.Split(v, ".")
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

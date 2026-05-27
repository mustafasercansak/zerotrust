package mfa

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	authmw "github.com/zerotrust/backend/pkg/middleware"
)

type Handler struct {
	svc          *Service
	rdb          *redis.Client
	recentWindow time.Duration
}

func NewHandler(svc *Service, rdb *redis.Client, recentWindow time.Duration) *Handler {
	return &Handler{svc: svc, rdb: rdb, recentWindow: recentWindow}
}

// POST /api/v1/mfa/setup — generate a new TOTP secret as a pending candidate.
// The active secret is untouched until the user verifies and calls /mfa/verify.
// If MFA is already enabled, the request body must include {"current_code":"..."}.
func (h *Handler) Setup(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		CurrentCode string `json:"current_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	result, err := h.svc.Setup(r.Context(), claims.UserID, claims.Email, req.CurrentCode)
	if err != nil {
		if err.Error() == "invalid_code" || err.Error() == "current_code_required" {
			writeError(w, http.StatusBadRequest, "invalid_code")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"otp_auth_url": result.OTPAuthURL,
		"secret":       result.Secret,
	})
}

// POST /api/v1/mfa/verify — verify a TOTP code and enable MFA
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := h.svc.VerifyAndEnable(r.Context(), claims.UserID, req.Code); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_code")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// POST /api/v1/mfa/disable — disable MFA (requires a valid current TOTP code)
func (h *Handler) Disable(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := h.svc.Disable(r.Context(), claims.UserID, req.Code); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_code")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// GET /api/v1/mfa/status — returns whether MFA is currently enabled
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	enabled := h.svc.IsEnabled(r.Context(), claims.UserID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"enabled": enabled})
}

// POST /api/v1/mfa/step-up — verifies a live TOTP code and marks this session
// as recently MFA-verified for sensitive operations.
func (h *Handler) StepUp(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.rdb == nil {
		writeError(w, http.StatusServiceUnavailable, "mfa_unavailable")
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if !h.svc.Validate(r.Context(), claims.UserID, req.Code) {
		writeError(w, http.StatusBadRequest, "invalid_code")
		return
	}

	rt, err := r.Cookie("refresh_token")
	if err != nil || rt.Value == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := authmw.MarkRecentMFACookie(r.Context(), h.rdb, claims.UserID, rt.Value, h.recentWindow); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

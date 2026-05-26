package mfa

import (
	"encoding/json"
	"net/http"

	authmw "github.com/zerotrust/backend/pkg/middleware"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
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
	// Ignore decode errors — current_code is optional for first-time setup.
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck

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

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

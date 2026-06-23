package mfa

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	authmw "github.com/zerotrust/backend/pkg/middleware"
)

type Handler struct {
	svc          mfaService
	rdb          *redis.Client
	recentWindow time.Duration
	notif        notifier
}

type mfaService interface {
	Setup(ctx context.Context, userID, email, currentCode string) (otpAuthURL, secret string, recoveryCodes []string, err error)
	VerifyAndEnable(ctx context.Context, userID, code string) error
	Disable(ctx context.Context, userID, code string) error
	IsEnabled(ctx context.Context, userID string) bool
	Validate(ctx context.Context, userID, code string) bool
}

type notifier interface {
	SendSecurityAlert(ctx context.Context, to, alertType, ipAddress, location, details string) error
}

func NewHandler(svc mfaService, rdb *redis.Client, recentWindow time.Duration) *Handler {
	return &Handler{svc: svc, rdb: rdb, recentWindow: recentWindow}
}

const (
	mfaMaxFailedAttempts = 5
	mfaAttemptWindow     = 10 * time.Minute
)

// mfaStepUpAttemptsKey intentionally matches the key used by RequireRecentMFA
// so that failed attempts from this handler and the middleware share one counter.
func mfaStepUpAttemptsKey(userID string) string { return "mfa:stepup:fails:" + userID }
func mfaDisableAttemptsKey(userID string) string { return "mfa:disable:fails:" + userID }

func mfaAttemptsExceeded(ctx context.Context, rdb *redis.Client, key string) bool {
	if rdb == nil {
		return false
	}
	n, err := rdb.Get(ctx, key).Int()
	if err != nil {
		return false
	}
	return n >= mfaMaxFailedAttempts
}

func recordMFAFailure(ctx context.Context, rdb *redis.Client, key string) {
	if rdb == nil {
		return
	}
	n, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return
	}
	if n == 1 {
		rdb.Expire(ctx, key, mfaAttemptWindow)
	}
}

func clearMFAFailures(ctx context.Context, rdb *redis.Client, key string) {
	if rdb == nil {
		return
	}
	rdb.Del(ctx, key)
}

func (h *Handler) ConfigureNotifier(n notifier) {
	h.notif = n
}

// clientIP returns the real client IP, which TrustedClientIP middleware has
// already resolved from proxy headers and written back into r.RemoteAddr.
func clientIP(r *http.Request) string {
	return authmw.ClientIP(r)
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
	otpAuthURL, secret, recoveryCodes, err := h.svc.Setup(r.Context(), claims.UserID, claims.Email, req.CurrentCode)
	if err != nil {
		if err.Error() == "invalid_code" || err.Error() == "current_code_required" {
			writeError(w, http.StatusBadRequest, "invalid_code")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"otp_auth_url":   otpAuthURL,
		"secret":         secret,
		"recovery_codes": recoveryCodes,
	})
}
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

	if h.notif != nil {
		_ = h.notif.SendSecurityAlert(r.Context(), claims.Email,
			"mfa_enabled", clientIP(r), "Unknown",
			"Two-factor authentication (TOTP) was enabled on your account.")
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

	disableFailKey := mfaDisableAttemptsKey(claims.UserID)
	if mfaAttemptsExceeded(r.Context(), h.rdb, disableFailKey) {
		writeError(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	if err := h.svc.Disable(r.Context(), claims.UserID, req.Code); err != nil {
		recordMFAFailure(r.Context(), h.rdb, disableFailKey)
		writeError(w, http.StatusBadRequest, "invalid_code")
		return
	}
	clearMFAFailures(r.Context(), h.rdb, disableFailKey)

	if h.notif != nil {
		_ = h.notif.SendSecurityAlert(r.Context(), claims.Email,
			"mfa_disabled", clientIP(r), "Unknown",
			"Two-factor authentication (TOTP) was disabled on your account.")
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
	json.NewEncoder(w).Encode(map[string]interface{}{"enabled": enabled, "supported": true})
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
		Code   string `json:"code"`
		Reason string `json:"reason,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	stepUpFailKey := mfaStepUpAttemptsKey(claims.UserID)
	if mfaAttemptsExceeded(r.Context(), h.rdb, stepUpFailKey) {
		writeError(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	if !h.svc.Validate(r.Context(), claims.UserID, req.Code) {
		recordMFAFailure(r.Context(), h.rdb, stepUpFailKey)
		writeError(w, http.StatusBadRequest, "invalid_code")
		return
	}
	clearMFAFailures(r.Context(), h.rdb, stepUpFailKey)
	if req.Reason != "" {
		if extras := authmw.AuditExtrasFrom(r.Context()); extras != nil {
			extras.Set("step_up_for", req.Reason)
		}
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

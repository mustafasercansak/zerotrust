package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/zerotrust/backend/internal/audit"
	"github.com/zerotrust/backend/internal/passwdreset"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/validation"
)

// PasswordResetter is implemented by passwdreset.Service.
type PasswordResetter interface {
	SendReset(ctx context.Context, email, baseURL string) error
	Reset(ctx context.Context, token, newPassword string) error
}

type auditLogger interface {
	Log(context.Context, audit.Entry) error
}

type authService interface {
	ClientCredentials(ctx context.Context, clientID, secret string, dpopJKT string) (*ServiceTokenResponse, error)
	Login(ctx context.Context, email, password, ip, ua string, deviceInfo map[string]string) (*LoginResult, error)
	MFAChallenge(ctx context.Context, pendingToken, totpCode string) (*TokenPair, error)
	RefreshTokens(ctx context.Context, refreshToken, ip, ua string, deviceInfo map[string]string) (*TokenPair, error)
	Logout(ctx context.Context, refreshToken, accessToken string) error
	ConsumeDPoPProof(ctx context.Context, jti string) error
	WebAuthnLoginBegin(ctx context.Context, pendingToken string) (json.RawMessage, error)
	WebAuthnLoginFinish(ctx context.Context, pendingToken string, credential []byte) (*TokenPair, error)
	WebAuthnPasswordlessBegin(ctx context.Context) (json.RawMessage, error)
	WebAuthnPasswordlessFinish(ctx context.Context, ceremonyID string, credential []byte, ip, ua string, deviceInfo map[string]string) (*TokenPair, error)
}

type userService interface {
	Register(ctx context.Context, email, password, locale string) (*user.User, error)
}

type Handler struct {
	authSvc             authService
	userSvc             userService
	auditRepo           auditLogger
	settings            SettingReader    // nil falls back to defaults
	passwordResetter    PasswordResetter // nil when not configured
	cookiesSecure       bool
	registrationEnabled bool
	publicAppURL        string // base URL for password-reset links (from config, never from request)
}

func NewHandler(authSvc authService, userSvc userService, auditRepo auditLogger, cookiesSecure, registrationEnabled bool, pr PasswordResetter, publicAppURL string, settings SettingReader) *Handler {
	return &Handler{
		authSvc:             authSvc,
		userSvc:             userSvc,
		auditRepo:           auditRepo,
		settings:            settings,
		passwordResetter:    pr,
		cookiesSecure:       cookiesSecure,
		registrationEnabled: registrationEnabled,
		publicAppURL:        publicAppURL,
	}
}

// POST /api/v1/auth/token — client_credentials grant (M2M, returns tokens in JSON body)
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GrantType    string `json:"grant_type"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.GrantType != "client_credentials" {
		writeError(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}
	if req.ClientID == "" || req.ClientSecret == "" {
		writeError(w, http.StatusBadRequest, "missing_fields")
		return
	}

	dpopProof := r.Header.Get("DPoP")
	var dpopJKT string
	if dpopProof != "" {
		jkt, jti, err := ValidateDPoPProofWithJTI(dpopProof, r.Method, "/api/v1/auth/token")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_dpop_proof")
			return
		}
		// Reject replayed proofs (same jti within the skew window). #35
		if err := h.authSvc.ConsumeDPoPProof(r.Context(), jti); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_dpop_proof")
			return
		}
		dpopJKT = jkt
	}

	resp, err := h.authSvc.ClientCredentials(r.Context(), req.ClientID, req.ClientSecret, dpopJKT)
	if err != nil {
		h.logAudit(r.Context(), audit.Entry{
			Action:    "auth.client_credentials_failed",
			Resource:  "/api/v1/auth/token",
			IPAddress: r.RemoteAddr,
			UserAgent: r.Header.Get("User-Agent"),
			Metadata:  serviceTokenMetadata(req.ClientID, "invalid_client", http.StatusUnauthorized),
		}, true)
		writeError(w, http.StatusUnauthorized, "invalid_client")
		return
	}

	h.logAudit(r.Context(), audit.Entry{
		Action:    "auth.client_credentials_success",
		Resource:  "/api/v1/auth/token",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  serviceTokenMetadata(req.ClientID, "", http.StatusOK),
	}, true)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token": resp.AccessToken,
		"token_type":   "bearer",
		"expires_in":   resp.ExpiresIn,
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email      string            `json:"email"`
		Password   string            `json:"password"`
		ClientInfo map[string]string `json:"client_info"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "missing_fields")
		return
	}

	clientInfo := req.ClientInfo
	if len(clientInfo) == 0 {
		clientInfo = parseXClientInfo(r)
	}

	result, err := h.authSvc.Login(r.Context(), req.Email, req.Password, r.RemoteAddr, r.Header.Get("User-Agent"), clientInfo)
	if err != nil {
		var lockedErr *AccountLockedError
		switch {
		case errors.As(err, &lockedErr):
			h.logAudit(r.Context(), audit.Entry{
				Action:    "auth.login_failed",
				Resource:  "/api/v1/auth/login",
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  authMetadata(req.Email, "account_locked", http.StatusTooManyRequests, clientInfo),
			}, true)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error":       "account_locked",
				"retry_after": int(lockedErr.RetryAfter.Seconds()),
			})
		case errors.Is(err, ErrIPNotAllowed):
			h.logAudit(r.Context(), audit.Entry{
				Action:    "auth.login_blocked",
				Resource:  "/api/v1/auth/login",
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  authMetadata(req.Email, "ip_not_allowed", http.StatusForbidden, clientInfo),
			}, true)
			writeError(w, http.StatusForbidden, "ip_not_allowed")
		case errors.Is(err, ErrCountryNotAllowed):
			h.logAudit(r.Context(), audit.Entry{
				Action:    "auth.login_blocked",
				Resource:  "/api/v1/auth/login",
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  authMetadata(req.Email, "country_not_allowed", http.StatusForbidden, clientInfo),
			}, true)
			writeError(w, http.StatusForbidden, "country_not_allowed")
		case errors.Is(err, ErrDeviceNotAllowed):
			h.logAudit(r.Context(), audit.Entry{
				Action:    "auth.login_blocked",
				Resource:  "/api/v1/auth/login",
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  authMetadata(req.Email, "device_not_allowed", http.StatusForbidden, clientInfo),
			}, true)
			writeError(w, http.StatusForbidden, "device_not_allowed")
		case errors.Is(err, ErrHighRiskBlocked):
			h.logAudit(r.Context(), audit.Entry{
				Action:    "auth.login_blocked",
				Resource:  "/api/v1/auth/login",
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  authMetadata(req.Email, "high_risk_blocked", http.StatusForbidden, clientInfo),
			}, true)
			writeError(w, http.StatusForbidden, "high_risk_blocked")
		case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrInactiveUser):
			h.logAudit(r.Context(), audit.Entry{
				Action:    "auth.login_failed",
				Resource:  "/api/v1/auth/login",
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  authMetadata(req.Email, err.Error(), http.StatusUnauthorized, clientInfo),
			}, true)
			writeError(w, http.StatusUnauthorized, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}

	h.logAudit(r.Context(), audit.Entry{
		Action:    "auth.login_success",
		Resource:  "/api/v1/auth/login",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  authMetadata(req.Email, "", http.StatusOK, req.ClientInfo),
	}, true)

	if result.AnomalyType != "" || result.RiskScore > 0 {
		h.logAudit(r.Context(), audit.Entry{
			Action:    "login.anomaly",
			Resource:  "/api/v1/auth/login",
			IPAddress: r.RemoteAddr,
			UserAgent: r.Header.Get("User-Agent"),
			Metadata: map[string]any{
				"email":        req.Email,
				"anomaly_type": result.AnomalyType,
				"details":      result.AnomalyDetails,
				"risk_score":   result.RiskScore,
				"outcome":      "success",
			},
		}, true)
	}

	if result.MFARequired {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"mfa_required":     true,
			"mfa_token":        result.MFAPendingToken,
			"totp_enabled":     result.TOTPEnabled,
			"webauthn_enabled": result.WebAuthnEnabled,
		}
		if result.MFASetupSecret != "" {
			resp["mfa_setup_secret"] = result.MFASetupSecret
			resp["mfa_setup_url"] = result.MFASetupURL
			resp["mfa_recovery_codes"] = result.MFARecoveryCodes
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	h.writeCookies(w, r, result.Pair)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func authMetadata(email, reason string, status int, clientInfo map[string]string) map[string]any {
	metadata := map[string]any{
		"email":   email,
		"status":  status,
		"outcome": auditOutcome(status),
	}
	if reason != "" {
		metadata["reason"] = reason
	}
	if len(clientInfo) > 0 {
		metadata["client_info"] = clientInfo
	}
	return metadata
}

func serviceTokenMetadata(clientID, reason string, status int) map[string]any {
	metadata := statusMetadata(reason, status)
	if clientID != "" {
		metadata["client_id"] = clientID
	}
	return metadata
}

func statusMetadata(reason string, status int) map[string]any {
	metadata := map[string]any{
		"status":  status,
		"outcome": auditOutcome(status),
	}
	if reason != "" {
		metadata["reason"] = reason
	}
	return metadata
}

// POST /api/v1/auth/mfa/challenge — second factor after Login returned mfa_required
func (h *Handler) MFAChallenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MFAToken string `json:"mfa_token"`
		TOTPCode string `json:"totp_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.MFAToken == "" || req.TOTPCode == "" {
		writeError(w, http.StatusBadRequest, "missing_fields")
		return
	}

	pair, err := h.authSvc.MFAChallenge(r.Context(), req.MFAToken, req.TOTPCode)
	if err != nil {
		h.logAudit(r.Context(), audit.Entry{
			Action:    "auth.mfa_challenge_failed",
			Resource:  "/api/v1/auth/mfa/challenge",
			IPAddress: r.RemoteAddr,
			UserAgent: r.Header.Get("User-Agent"),
			Metadata:  statusMetadata("invalid_credentials", http.StatusUnauthorized),
		}, true)
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	h.logAudit(r.Context(), audit.Entry{
		Action:    "auth.mfa_challenge_success",
		Resource:  "/api/v1/auth/mfa/challenge",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  statusMetadata("", http.StatusOK),
	}, true)

	h.writeCookies(w, r, pair)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// POST /api/v1/auth/webauthn/login/begin — passkey assertion options for the
// second factor. Body: {"mfa_token":"..."}
func (h *Handler) WebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MFAToken string `json:"mfa_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MFAToken == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	opts, err := h.authSvc.WebAuthnLoginBegin(r.Context(), req.MFAToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(opts)
}

// POST /api/v1/auth/webauthn/login/finish — verify the passkey assertion and
// complete login. Body: {"mfa_token":"...","credential":{...assertion...}}
func (h *Handler) WebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MFAToken   string          `json:"mfa_token"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MFAToken == "" || len(req.Credential) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	pair, err := h.authSvc.WebAuthnLoginFinish(r.Context(), req.MFAToken, req.Credential)
	if err != nil {
		h.logAudit(r.Context(), audit.Entry{
			Action:    "auth.webauthn_login_failed",
			Resource:  "/api/v1/auth/webauthn/login/finish",
			IPAddress: r.RemoteAddr,
			UserAgent: r.Header.Get("User-Agent"),
			Metadata:  statusMetadata("invalid_credentials", http.StatusUnauthorized),
		}, true)
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	h.logAudit(r.Context(), audit.Entry{
		Action:    "auth.webauthn_login_success",
		Resource:  "/api/v1/auth/webauthn/login/finish",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  statusMetadata("", http.StatusOK),
	}, true)

	h.writeCookies(w, r, pair)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// POST /api/v1/auth/webauthn/passwordless/begin — assertion options for a
// passwordless (usernameless) passkey login. No request body required.
func (h *Handler) WebAuthnPasswordlessBegin(w http.ResponseWriter, r *http.Request) {
	opts, err := h.authSvc.WebAuthnPasswordlessBegin(r.Context())
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(opts)
}

// POST /api/v1/auth/webauthn/passwordless/finish — verify the passwordless
// assertion and complete login. Body: {"ceremony_id":"...","credential":{...}}
func (h *Handler) WebAuthnPasswordlessFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CeremonyID string          `json:"ceremony_id"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CeremonyID == "" || len(req.Credential) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	clientInfo := parseXClientInfo(r)
	pair, err := h.authSvc.WebAuthnPasswordlessFinish(r.Context(), req.CeremonyID, req.Credential, r.RemoteAddr, r.Header.Get("User-Agent"), clientInfo)
	if err != nil {
		if errors.Is(err, ErrIPNotAllowed) {
			h.logAudit(r.Context(), audit.Entry{
				Action:    "auth.login_blocked",
				Resource:  "/api/v1/auth/webauthn/passwordless/finish",
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  statusMetadata("ip_not_allowed", http.StatusForbidden),
			}, true)
			writeError(w, http.StatusForbidden, "ip_not_allowed")
			return
		}
		if errors.Is(err, ErrCountryNotAllowed) {
			h.logAudit(r.Context(), audit.Entry{
				Action:    "auth.login_blocked",
				Resource:  "/api/v1/auth/webauthn/passwordless/finish",
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  statusMetadata("country_not_allowed", http.StatusForbidden),
			}, true)
			writeError(w, http.StatusForbidden, "country_not_allowed")
			return
		}
		if errors.Is(err, ErrDeviceNotAllowed) {
			h.logAudit(r.Context(), audit.Entry{
				Action:    "auth.login_blocked",
				Resource:  "/api/v1/auth/webauthn/passwordless/finish",
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  authMetadata("", "device_not_allowed", http.StatusForbidden, clientInfo),
			}, true)
			writeError(w, http.StatusForbidden, "device_not_allowed")
			return
		}
		h.logAudit(r.Context(), audit.Entry{
			Action:    "auth.webauthn_passwordless_failed",
			Resource:  "/api/v1/auth/webauthn/passwordless/finish",
			IPAddress: r.RemoteAddr,
			UserAgent: r.Header.Get("User-Agent"),
			Metadata:  statusMetadata("invalid_credentials", http.StatusUnauthorized),
		}, true)
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	h.logAudit(r.Context(), audit.Entry{
		Action:    "auth.webauthn_passwordless_success",
		Resource:  "/api/v1/auth/webauthn/passwordless/finish",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  statusMetadata("", http.StatusOK),
	}, true)

	h.writeCookies(w, r, pair)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientInfo map[string]string `json:"client_info"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	c, err := r.Cookie("refresh_token")
	if err != nil || c.Value == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	pair, err := h.authSvc.RefreshTokens(r.Context(), c.Value, r.RemoteAddr, r.Header.Get("User-Agent"), req.ClientInfo)
	if err != nil {
		h.logAudit(r.Context(), audit.Entry{
			Action:    "auth.refresh_failed",
			Resource:  "/api/v1/auth/refresh",
			IPAddress: r.RemoteAddr,
			UserAgent: r.Header.Get("User-Agent"),
			Metadata:  statusMetadata("invalid_token", http.StatusUnauthorized),
		}, true)
		h.clearCookies(w)
		writeError(w, http.StatusUnauthorized, "invalid_token")
		return
	}

	h.writeCookies(w, r, pair)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	rt := ""
	at := ""
	if c, err := r.Cookie("refresh_token"); err == nil {
		rt = c.Value
	}
	if c, err := r.Cookie("access_token"); err == nil {
		at = c.Value
	}
	if rt != "" || at != "" {
		h.authSvc.Logout(r.Context(), rt, at)
	}
	h.logAudit(r.Context(), audit.Entry{
		Action:    "auth.logout",
		Resource:  "/api/v1/auth/logout",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  auditMetadataFromHeader(r, http.StatusNoContent),
	}, true)
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.registrationEnabled {
		writeError(w, http.StatusForbidden, "registration_disabled")
		return
	}

	var req struct {
		Email      string            `json:"email"`
		Password   string            `json:"password"`
		Locale     string            `json:"locale"`
		ClientInfo map[string]string `json:"client_info"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := validation.Email(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	complexity := "low"
	if h.settings != nil {
		complexity = h.settings.GetString(r.Context(), "password_complexity", "low")
	}
	if err := validation.PasswordWithComplexity(req.Password, complexity); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.Locale == "" {
		req.Locale = "tr"
	}

	u, err := h.userSvc.Register(r.Context(), req.Email, req.Password, req.Locale)
	if err != nil {
		if errors.Is(err, user.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "email_taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	clientInfo := req.ClientInfo
	if len(clientInfo) == 0 {
		clientInfo = parseXClientInfo(r)
	}

	h.logAudit(r.Context(), audit.Entry{
		UserID:    &u.ID,
		Action:    "auth.register",
		Resource:  "/api/v1/auth/register",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  authMetadata(req.Email, "", http.StatusCreated, clientInfo),
	}, false)

	result, err := h.authSvc.Login(r.Context(), req.Email, req.Password, r.RemoteAddr, r.Header.Get("User-Agent"), clientInfo)
	if err != nil {
		if errors.Is(err, ErrIPNotAllowed) {
			writeError(w, http.StatusForbidden, "ip_not_allowed")
			return
		}
		if errors.Is(err, ErrCountryNotAllowed) {
			writeError(w, http.StatusForbidden, "country_not_allowed")
			return
		}
		if errors.Is(err, ErrDeviceNotAllowed) {
			writeError(w, http.StatusForbidden, "device_not_allowed")
			return
		}
		if errors.Is(err, ErrHighRiskBlocked) {
			writeError(w, http.StatusForbidden, "high_risk_blocked")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	if !result.MFARequired {
		h.writeCookies(w, r, result.Pair)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// POST /api/v1/auth/forgot-password
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if h.passwordResetter == nil {
		writeError(w, http.StatusNotImplemented, "not_configured")
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	// Always respond 200 — never reveal whether the email exists.
	go h.passwordResetter.SendReset(context.Background(), req.Email, h.publicAppURL)
	h.logAudit(r.Context(), audit.Entry{
		Action:    "auth.password_reset_requested",
		Resource:  "/api/v1/auth/forgot-password",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  authMetadata(req.Email, "", http.StatusOK, nil),
	}, true)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// POST /api/v1/auth/reset-password
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if h.passwordResetter == nil {
		writeError(w, http.StatusNotImplemented, "not_configured")
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "missing_fields")
		return
	}

	complexity := "low"
	if h.settings != nil {
		complexity = h.settings.GetString(r.Context(), "password_complexity", "low")
	}
	if err := validation.PasswordWithComplexity(req.Password, complexity); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.passwordResetter.Reset(r.Context(), req.Token, req.Password); err != nil {
		auditErr := "invalid_token"
		if errors.Is(err, passwdreset.ErrPasswordReuseForbidden) {
			auditErr = "password_reuse_forbidden"
		}
		h.logAudit(r.Context(), audit.Entry{
			Action:    "auth.password_reset_failed",
			Resource:  "/api/v1/auth/reset-password",
			IPAddress: r.RemoteAddr,
			UserAgent: r.Header.Get("User-Agent"),
			Metadata:  statusMetadata(auditErr, http.StatusBadRequest),
		}, true)
		writeError(w, http.StatusBadRequest, auditErr)
		return
	}
	h.logAudit(r.Context(), audit.Entry{
		Action:    "auth.password_reset_success",
		Resource:  "/api/v1/auth/reset-password",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  statusMetadata("", http.StatusOK),
	}, true)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// writeCookies sets the four session cookies: access_token (httpOnly), refresh_token (httpOnly),
// csrf_token (JS-readable, double-submit pattern), at_exp (JS-readable, for refresh scheduling).
func (h *Handler) writeCookies(w http.ResponseWriter, r *http.Request, pair *TokenPair) {
	secure := h.cookiesSecure
	expAt := time.Now().Add(AccessTTL)

	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    pair.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(AccessTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    pair.RefreshToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(RefreshTTL.Seconds()),
	})
	// csrf_token: reuse existing cookie value if present and valid to prevent rotation races
	// We only reuse it during background token refresh to avoid race conditions. For initial login,
	// registration, and MFA verification we always rotate the CSRF token to prevent session fixation.
	csrfVal := ""
	if r != nil && r.URL.Path == "/api/v1/auth/refresh" {
		if c, err := r.Cookie("csrf_token"); err == nil && c.Value != "" {
			csrfVal = c.Value
		}
	}
	if csrfVal == "" {
		csrfVal = newCSRFToken()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    csrfVal,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(RefreshTTL.Seconds()),
	})
	// at_exp: Unix timestamp JS reads to know when to schedule the next refresh
	http.SetCookie(w, &http.Cookie{
		Name:     "at_exp",
		Value:    strconv.FormatInt(expAt.Unix(), 10),
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(AccessTTL.Seconds()),
	})
}

func (h *Handler) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{"access_token", "refresh_token", "csrf_token", "at_exp"} {
		http.SetCookie(w, &http.Cookie{
			Name:   name,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
	}
}

func newCSRFToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// logAudit writes critical audit entries synchronously and non-critical entries
// in a goroutine decoupled from the HTTP request lifetime. context.WithoutCancel
// ensures async writes are not aborted when the handler returns; the 5-second
// timeout prevents unbounded waits on slow DB paths.
func (h *Handler) logAudit(parent context.Context, entry audit.Entry, synchronous bool) {
	if h.auditRepo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	write := func() {
		if err := h.auditRepo.Log(ctx, entry); err != nil {
			slog.Error("audit log write failed",
				"error", err,
				"action", entry.Action,
				"resource", entry.Resource,
			)
			audit.RecordWriteFailure()
		}
	}
	if synchronous {
		defer cancel()
		write()
		return
	}
	go func() {
		defer cancel()
		write()
	}()
}

func auditMetadataFromHeader(r *http.Request, status int) map[string]any {
	metadata := map[string]any{
		"status":  status,
		"outcome": auditOutcome(status),
	}
	raw := r.Header.Get("X-Client-Info")
	if raw == "" {
		return metadata
	}
	var clientInfo map[string]string
	if err := json.Unmarshal([]byte(raw), &clientInfo); err != nil || len(clientInfo) == 0 {
		return metadata
	}
	metadata["client_info"] = clientInfo
	return metadata
}

func auditOutcome(status int) string {
	if status >= 200 && status < 400 {
		return "success"
	}
	return "failure"
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func parseXClientInfo(r *http.Request) map[string]string {
	raw := r.Header.Get("X-Client-Info")
	if raw == "" {
		return nil
	}
	var clientInfo map[string]string
	if err := json.Unmarshal([]byte(raw), &clientInfo); err != nil {
		return nil
	}
	return clientInfo
}

package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/zerotrust/backend/internal/audit"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/validation"
)

// PasswordResetter is implemented by passwdreset.Service.
type PasswordResetter interface {
	SendReset(ctx context.Context, email, baseURL string) error
	Reset(ctx context.Context, token, newPassword string) error
}

type Handler struct {
	authSvc             *Service
	userSvc             *user.Service
	auditRepo           *audit.Repository
	passwordResetter    PasswordResetter // nil when not configured
	cookiesSecure       bool
	registrationEnabled bool
	publicAppURL        string // base URL for password-reset links (from config, never from request)
}

func NewHandler(authSvc *Service, userSvc *user.Service, auditRepo *audit.Repository, cookiesSecure, registrationEnabled bool, pr PasswordResetter, publicAppURL string) *Handler {
	return &Handler{
		authSvc:             authSvc,
		userSvc:             userSvc,
		auditRepo:           auditRepo,
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

	resp, err := h.authSvc.ClientCredentials(r.Context(), req.ClientID, req.ClientSecret)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_client")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token": resp.AccessToken,
		"token_type":   "bearer",
		"expires_in":   resp.ExpiresIn,
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "missing_fields")
		return
	}

	result, err := h.authSvc.Login(r.Context(), req.Email, req.Password, r.RemoteAddr, r.Header.Get("User-Agent"))
	if err != nil {
		var lockedErr *AccountLockedError
		switch {
		case errors.As(err, &lockedErr):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error":       "account_locked",
				"retry_after": int(lockedErr.RetryAfter.Seconds()),
			})
		case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrInactiveUser):
			h.asyncLog(r.Context(), audit.Entry{
				Action:    "auth.login_failed",
				Resource:  "/api/v1/auth/login",
				IPAddress: r.RemoteAddr,
				UserAgent: r.Header.Get("User-Agent"),
				Metadata:  map[string]any{"email": req.Email, "reason": err.Error()},
			})
			writeError(w, http.StatusUnauthorized, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}

	h.asyncLog(r.Context(), audit.Entry{
		Action:    "auth.login_success",
		Resource:  "/api/v1/auth/login",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  map[string]any{"email": req.Email},
	})

	if result.MFARequired {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"mfa_required": true,
			"mfa_token":    result.MFAPendingToken,
		})
		return
	}

	h.writeCookies(w, result.Pair)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
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
		writeError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	h.writeCookies(w, pair)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("refresh_token")
	if err != nil || c.Value == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	pair, err := h.authSvc.RefreshTokens(r.Context(), c.Value, r.RemoteAddr, r.Header.Get("User-Agent"))
	if err != nil {
		h.clearCookies(w)
		writeError(w, http.StatusUnauthorized, "invalid_token")
		return
	}

	h.writeCookies(w, pair)
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
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.registrationEnabled {
		writeError(w, http.StatusForbidden, "registration_disabled")
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Locale   string `json:"locale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := validation.Email(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validation.Password(req.Password); err != nil {
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

	h.asyncLog(r.Context(), audit.Entry{
		UserID:    &u.ID,
		Action:    "auth.register",
		Resource:  "/api/v1/auth/register",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
	})

	result, err := h.authSvc.Login(r.Context(), req.Email, req.Password, r.RemoteAddr, r.Header.Get("User-Agent"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if result.MFARequired {
		// Can't happen for a brand-new account; handle gracefully.
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}
	h.writeCookies(w, result.Pair)
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
	if err := validation.Password(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.passwordResetter.Reset(r.Context(), req.Token, req.Password); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_token")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// writeCookies sets the four session cookies: access_token (httpOnly), refresh_token (httpOnly),
// csrf_token (JS-readable, double-submit pattern), at_exp (JS-readable, for refresh scheduling).
func (h *Handler) writeCookies(w http.ResponseWriter, pair *TokenPair) {
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
	// csrf_token: long-lived so it survives multiple access token refresh cycles
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    newCSRFToken(),
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

// asyncLog fires an audit log entry in a goroutine that is decoupled from the
// HTTP request lifetime but still inherits its values (trace IDs, etc.).
// context.WithoutCancel ensures the log write is not aborted when the handler
// returns; the 5-second timeout prevents goroutine leaks on slow DB paths.
func (h *Handler) asyncLog(parent context.Context, entry audit.Entry) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	go func() {
		defer cancel()
		h.auditRepo.Log(ctx, entry)
	}()
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

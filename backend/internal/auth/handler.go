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

type Handler struct {
	authSvc       *Service
	userSvc       *user.Service
	auditRepo     *audit.Repository
	cookiesSecure bool
}

func NewHandler(authSvc *Service, userSvc *user.Service, auditRepo *audit.Repository, cookiesSecure bool) *Handler {
	return &Handler{
		authSvc:       authSvc,
		userSvc:       userSvc,
		auditRepo:     auditRepo,
		cookiesSecure: cookiesSecure,
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

	pair, err := h.authSvc.Login(r.Context(), req.Email, req.Password, r.RemoteAddr, r.Header.Get("User-Agent"))
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
			go h.auditRepo.Log(context.Background(), audit.Entry{
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

	go h.auditRepo.Log(context.Background(), audit.Entry{
		Action:    "auth.login_success",
		Resource:  "/api/v1/auth/login",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  map[string]any{"email": req.Email},
	})

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

	go h.auditRepo.Log(context.Background(), audit.Entry{
		UserID:    &u.ID,
		Action:    "auth.register",
		Resource:  "/api/v1/auth/register",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
	})

	pair, err := h.authSvc.Login(r.Context(), req.Email, req.Password, r.RemoteAddr, r.Header.Get("User-Agent"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	h.writeCookies(w, pair)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
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

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

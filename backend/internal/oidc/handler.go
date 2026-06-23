package oidc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/zerotrust/backend/internal/audit"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/user"
	authmw "github.com/zerotrust/backend/pkg/middleware"
	"golang.org/x/crypto/bcrypt"
)

type auditLogger interface {
	Log(context.Context, audit.Entry) error
}

// consentMFAGuard is satisfied by *mfa.Service. Defined locally to avoid an
// import cycle and to keep the interface minimal (only IsEnabled is needed).
type consentMFAGuard interface {
	IsEnabled(ctx context.Context, userID string) bool
}

// clientStore is the full set of ClientRepository methods used by the Handler.
// *ClientRepository satisfies it; tests may supply a stub.
type clientStore interface {
	FindByClientID(ctx context.Context, clientID string) (*Client, error)
	AuthenticateClient(ctx context.Context, clientID, clientSecret string) (*Client, error)
	List(ctx context.Context) ([]*Client, error)
	Create(ctx context.Context, clientID, secretHash, name string, redirectURIs, allowedScopes []string) (*Client, error)
	Delete(ctx context.Context, id string) error
	Update(ctx context.Context, id, name string, redirectURIs, allowedScopes []string) (*Client, error)
	RotateSecret(ctx context.Context, id string) (string, error)
}

type Handler struct {
	svc          *Service
	clientRepo   clientStore
	userSvc      *user.Service
	authSvc      *auth.Service
	ks           *auth.KeyStore
	issuer       string
	publicAppURL string
	auditRepo    auditLogger
	mfaSvc       consentMFAGuard
	rdb          *redis.Client
}

func NewHandler(svc *Service, clientRepo *ClientRepository, userSvc *user.Service, authSvc *auth.Service, ks *auth.KeyStore, issuer string, publicAppURL string, auditRepo auditLogger, mfaSvc consentMFAGuard, rdb *redis.Client) *Handler {
	return &Handler{
		svc:          svc,
		clientRepo:   clientRepo,
		userSvc:      userSvc,
		authSvc:      authSvc,
		ks:           ks,
		issuer:       issuer,
		publicAppURL: publicAppURL,
		auditRepo:    auditRepo,
		mfaSvc:       mfaSvc,
		rdb:          rdb,
	}
}

func (h *Handler) logAudit(parent context.Context, entry audit.Entry) {
	if h.auditRepo == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	go func() {
		defer cancel()
		if err := h.auditRepo.Log(ctx, entry); err != nil {
			slog.Error("oidc audit log write failed", "error", err, "action", entry.Action)
			audit.RecordWriteFailure()
		}
	}()
}

// authorizeRedirectError sends an OAuth2 error response to the client's
// redirect_uri rather than returning an HTTP error, per RFC 6749 §4.1.2.1.
func authorizeRedirectError(w http.ResponseWriter, r *http.Request, redirectURI, errCode, state string) {
	parsed, _ := url.Parse(redirectURI)
	q := parsed.Query()
	q.Set("error", errCode)
	if state != "" {
		q.Set("state", state)
	}
	parsed.RawQuery = q.Encode()
	http.Redirect(w, r, parsed.String(), http.StatusFound)
}

// Authorize handles the initial GET /oauth2/authorize endpoint
func (h *Handler) Authorize(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	responseType := r.URL.Query().Get("response_type")

	if clientID == "" || redirectURI == "" || responseType != "code" {
		http.Error(w, `{"error":"invalid_request","error_description":"client_id, redirect_uri and response_type=code are required"}`, http.StatusBadRequest)
		return
	}

	client, err := h.clientRepo.FindByClientID(r.Context(), clientID)
	if err != nil {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}

	if !client.ValidateRedirectURI(redirectURI) {
		http.Error(w, `{"error":"invalid_redirect_uri"}`, http.StatusBadRequest)
		return
	}

	prompt := r.URL.Query().Get("prompt")
	state := r.URL.Query().Get("state")

	// prompt=login: force re-authentication regardless of existing session.
	if prompt == "login" {
		loginURL := h.publicAppURL + "/auth/login?redirect_to=" + url.QueryEscape(r.RequestURI)
		http.Redirect(w, r, loginURL, http.StatusFound)
		return
	}

	// Manually authenticate user session using the access_token cookie
	var claims *auth.Claims
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		validated, vErr := auth.ValidateAccessToken(h.ks, c.Value)
		if vErr == nil && (h.authSvc == nil || !h.authSvc.IsRevoked(r.Context(), validated.ID)) {
			claims = validated
		}
	}

	if claims == nil || claims.UserID == "" {
		if prompt == "none" {
			// prompt=none: must not show any UI. Return login_required to the client.
			authorizeRedirectError(w, r, redirectURI, "login_required", state)
			return
		}
		loginURL := h.publicAppURL + "/auth/login?redirect_to=" + url.QueryEscape(r.RequestURI)
		http.Redirect(w, r, loginURL, http.StatusFound)
		return
	}

	// OIDC Core §3.1.2.1: if max_age is specified and the session is at least
	// that many seconds old, force re-authentication. Use duration comparison
	// so max_age=0 ("must just have authenticated") always triggers re-auth.
	if maxAgeStr := r.URL.Query().Get("max_age"); maxAgeStr != "" {
		if maxAge, err := strconv.ParseInt(maxAgeStr, 10, 64); err == nil && maxAge >= 0 {
			if time.Since(claims.IssuedAt.Time) >= time.Duration(maxAge)*time.Second {
				if prompt == "none" {
					authorizeRedirectError(w, r, redirectURI, "login_required", state)
					return
				}
				loginURL := h.publicAppURL + "/auth/login?redirect_to=" + url.QueryEscape(r.RequestURI)
				http.Redirect(w, r, loginURL, http.StatusFound)
				return
			}
		}
	}

	// prompt=none: user is authenticated but we always require explicit consent
	// interaction, so return interaction_required per OIDC Core §3.1.2.6.
	if prompt == "none" {
		authorizeRedirectError(w, r, redirectURI, "interaction_required", state)
		return
	}

	// User is logged in. Redirect to the frontend consent screen
	consentURL := h.publicAppURL + "/oauth2/consent?" + r.URL.RawQuery
	http.Redirect(w, r, consentURL, http.StatusFound)
}

type ConsentRequest struct {
	ClientID            string   `json:"client_id"`
	RedirectURI         string   `json:"redirect_uri"`
	Scopes              []string `json:"scopes"`
	CodeChallenge       string   `json:"code_challenge"`
	CodeChallengeMethod string   `json:"code_challenge_method"`
	Nonce               string   `json:"nonce"`
	State               string   `json:"state"`
	Approved            bool     `json:"approved"`
}

// Consent processes user consent POST /oauth2/consent
func (h *Handler) Consent(w http.ResponseWriter, r *http.Request) {
	// The consent API uses the standard JSON/API flow, so we can require auth middleware.
	// We read claims populated by auth middleware.
	var claims *auth.Claims
	// Also fallback to manual cookie lookup if the middleware isn't mounted
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		validated, vErr := auth.ValidateAccessToken(h.ks, c.Value)
		if vErr == nil && (h.authSvc == nil || !h.authSvc.IsRevoked(r.Context(), validated.ID)) {
			claims = validated
		}
	}

	if claims == nil || claims.UserID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req ConsentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	// Step-up MFA guard: if the user has MFA enabled, require a recent proof
	// tied to their current session before approving third-party access.
	if req.Approved && h.mfaSvc != nil && h.rdb != nil {
		if h.mfaSvc.IsEnabled(r.Context(), claims.UserID) {
			rt, rtErr := r.Cookie("refresh_token")
			if rtErr != nil || !authmw.HasRecentMFACookie(r.Context(), h.rdb, claims.UserID, rt.Value) {
				http.Error(w, `{"error":"mfa_required"}`, http.StatusForbidden)
				return
			}
		}
	}

	// Look up client and validate redirect_uri before using it in any redirect,
	// including the denial path. Without this, a caller could supply an arbitrary
	// redirect_uri and turn the denial response into an open redirect.
	client, err := h.clientRepo.FindByClientID(r.Context(), req.ClientID)
	if err != nil {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}
	if !client.ValidateRedirectURI(req.RedirectURI) {
		http.Error(w, `{"error":"invalid_redirect_uri"}`, http.StatusBadRequest)
		return
	}

	if !req.Approved {
		h.logAudit(r.Context(), audit.Entry{
			UserID:    &claims.UserID,
			Action:    "oidc.consent_denied",
			Resource:  "oauth2",
			IPAddress: r.RemoteAddr,
			UserAgent: r.Header.Get("User-Agent"),
			Metadata:  map[string]any{"client_id": req.ClientID},
		})
		parsed, _ := url.Parse(req.RedirectURI)
		q := parsed.Query()
		q.Set("error", "access_denied")
		if req.State != "" {
			q.Set("state", req.State)
		}
		parsed.RawQuery = q.Encode()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"redirect_url": parsed.String()})
		return
	}

	// Validate scopes against client allowed scopes
	if !client.ValidateScope(req.Scopes) {
		http.Error(w, `{"error":"invalid_scope"}`, http.StatusBadRequest)
		return
	}

	// Create Auth Code
	code, err := h.svc.CreateAuthCodeSession(
		r.Context(),
		claims.UserID,
		req.ClientID,
		req.RedirectURI,
		req.Scopes,
		req.CodeChallenge,
		req.CodeChallengeMethod,
		req.Nonce,
	)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "server_error", "error_description": err.Error()})
		return
	}

	h.logAudit(r.Context(), audit.Entry{
		UserID:    &claims.UserID,
		Action:    "oidc.consent_approved",
		Resource:  "oauth2",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  map[string]any{"client_id": req.ClientID, "scopes": req.Scopes},
	})

	parsed, _ := url.Parse(req.RedirectURI)
	q := parsed.Query()
	q.Set("code", code)
	if req.State != "" {
		q.Set("state", req.State)
	}
	parsed.RawQuery = q.Encode()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"redirect_url": parsed.String()})
}

// Token handles code-to-token swap POST /oauth2/token
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	grantType := r.FormValue("grant_type")

	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	if clientID == "" {
		if id, secret, ok := r.BasicAuth(); ok {
			clientID, clientSecret = id, secret
		}
	}

	var resp *TokenResponse
	var err error

	switch grantType {
	case "authorization_code":
		code := r.FormValue("code")
		redirectURI := r.FormValue("redirect_uri")
		codeVerifier := r.FormValue("code_verifier")
		if code == "" || clientID == "" || redirectURI == "" {
			http.Error(w, `{"error":"invalid_request","error_description":"code, client_id, and redirect_uri are required"}`, http.StatusBadRequest)
			return
		}
		resp, err = h.svc.ExchangeCode(r.Context(), code, clientID, clientSecret, redirectURI, codeVerifier)

	case "refresh_token":
		rt := r.FormValue("refresh_token")
		if rt == "" || clientID == "" {
			http.Error(w, `{"error":"invalid_request","error_description":"refresh_token and client_id are required"}`, http.StatusBadRequest)
			return
		}
		var reqScopes []string
		if s := r.FormValue("scope"); s != "" {
			reqScopes = strings.Fields(s)
		}
		resp, err = h.svc.ExchangeRefreshToken(r.Context(), rt, clientID, clientSecret, reqScopes)

	default:
		http.Error(w, `{"error":"unsupported_grant_type"}`, http.StatusBadRequest)
		return
	}

	if err != nil {
		h.logAudit(r.Context(), audit.Entry{
			Action:    "oidc.token_exchange_failed",
			Resource:  "oauth2",
			IPAddress: r.RemoteAddr,
			Metadata:  map[string]any{"client_id": clientID, "reason": err.Error(), "grant_type": grantType},
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": err.Error()})
		return
	}

	h.logAudit(r.Context(), audit.Entry{
		Action:    "oidc.token_issued",
		Resource:  "oauth2",
		IPAddress: r.RemoteAddr,
		Metadata:  map[string]any{"client_id": clientID, "scope": resp.Scope, "grant_type": grantType},
	})

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	json.NewEncoder(w).Encode(resp)
}

// UserInfo returns OIDC UserInfo claims, filtered to the scopes granted in the
// access token (OIDC Core §5.3). Tokens that carry no scope claim (e.g. internal
// browser session tokens) receive the full response for backward compatibility.
func (h *Handler) UserInfo(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	claims, err := auth.ValidateAccessToken(h.ks, tokenStr)
	if err != nil || (h.authSvc != nil && h.authSvc.IsRevoked(r.Context(), claims.ID)) {
		http.Error(w, `{"error":"invalid_token"}`, http.StatusUnauthorized)
		return
	}

	u, err := h.userSvc.FindByID(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, `{"error":"invalid_token"}`, http.StatusUnauthorized)
		return
	}

	// Build scope lookup map. Empty Scopes means the token is an internal browser
	// session token that pre-dates scope tracking; return full info for compat.
	scopeSet := make(map[string]bool, len(claims.Scopes))
	for _, s := range claims.Scopes {
		scopeSet[s] = true
	}
	oidcToken := len(scopeSet) > 0

	resp := map[string]any{"sub": u.ID}

	includeProfile := !oidcToken || scopeSet["profile"]
	includeEmail := !oidcToken || scopeSet["email"]

	if includeProfile {
		resp["name"] = strings.TrimSpace(u.FirstName + " " + u.LastName)
		resp["given_name"] = u.FirstName
		resp["family_name"] = u.LastName
		resp["locale"] = u.Locale
		resp["roles"] = u.Roles
		resp["groups"] = u.Roles
	}
	if includeEmail {
		resp["email"] = u.Email
		resp["email_verified"] = true
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Discovery returns the OpenID Connect discovery document
func (h *Handler) Discovery(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"issuer":                                h.issuer,
		"authorization_endpoint":                h.issuer + "/oauth2/authorize",
		"token_endpoint":                        h.issuer + "/oauth2/token",
		"userinfo_endpoint":                     h.issuer + "/oauth2/userinfo",
		"jwks_uri":                              h.issuer + "/.well-known/jwks.json",
		"scopes_supported":                      []string{"openid", "profile", "email", "offline_access"},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"subject_types_supported":               []string{"public"},
		"revocation_endpoint":                   h.issuer + "/oauth2/revoke",
		"introspection_endpoint":                h.issuer + "/oauth2/introspect",
		"id_token_signing_alg_values_supported": []string{"EdDSA"},
		"code_challenge_methods_supported":      []string{"S256"},
		"end_session_endpoint":                   h.issuer + "/oauth2/end_session",
		"prompt_values_supported":               []string{"none", "login", "consent"},
		"request_uri_parameter_supported":       false,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Revoke handles POST /oauth2/revoke (RFC 7009).
// The client authenticates via HTTP Basic auth or form params. The token is
// immediately blocked in Redis via its JTI. Per the spec, invalid or already-
// expired tokens are silently ignored and a 200 is always returned to avoid
// leaking information about token state.
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	if clientID == "" {
		if id, secret, ok := r.BasicAuth(); ok {
			clientID, clientSecret = id, secret
		}
	}
	if clientID == "" {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}

	if _, err := h.clientRepo.AuthenticateClient(r.Context(), clientID, clientSecret); err != nil {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}

	if token := r.FormValue("token"); token != "" {
		hint := r.FormValue("token_type_hint")
		// Route by hint first; fall back to trying JWT parse to discriminate.
		if hint == "refresh_token" {
			h.svc.RevokeRefreshToken(r.Context(), token)
		} else {
			// Try to parse as a JWT access token. If it succeeds, blocklist the
			// JTI. Otherwise treat it as an opaque refresh token.
			if h.authSvc != nil {
				if claims, err := auth.ValidateAccessToken(h.ks, token); err == nil {
					_ = claims // validated; RevokeAccessToken re-parses internally
					h.authSvc.RevokeAccessToken(r.Context(), token)
				} else {
					h.svc.RevokeRefreshToken(r.Context(), token)
				}
			} else {
				h.svc.RevokeRefreshToken(r.Context(), token)
			}
		}
		h.logAudit(r.Context(), audit.Entry{
			Action:    "oidc.token_revoked",
			Resource:  "oauth2",
			IPAddress: r.RemoteAddr,
			Metadata:  map[string]any{"client_id": clientID, "token_type_hint": hint},
		})
	}

	w.WriteHeader(http.StatusOK)
}

// GetPublicClient returns the display name and allowed scopes for a client_id.
// This is intentionally unauthenticated so the consent page can show the real
// client name before the user approves.
func (h *Handler) GetPublicClient(w http.ResponseWriter, r *http.Request) {
	clientID := chi.URLParam(r, "client_id")
	client, err := h.clientRepo.FindByClientID(r.Context(), clientID)
	if err != nil {
		http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"name":           client.Name,
		"allowed_scopes": client.AllowedScopes,
	})
}

// ListClients returns all registered OIDC clients (admin only)
func (h *Handler) ListClients(w http.ResponseWriter, r *http.Request) {
	list, err := h.clientRepo.List(r.Context())
	if err != nil {
		http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// CreateClient registers a new OIDC client, returning the client credentials with the plaintext secret once (admin only)
func (h *Handler) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientID      string   `json:"client_id"`
		Name          string   `json:"name"`
		RedirectURIs  []string `json:"redirect_uris"`
		AllowedScopes []string `json:"allowed_scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	if req.ClientID == "" || req.Name == "" || len(req.RedirectURIs) == 0 {
		http.Error(w, `{"error":"invalid_request","error_description":"client_id, name, and redirect_uris are required"}`, http.StatusBadRequest)
		return
	}

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
		return
	}
	secret := hex.EncodeToString(secretBytes)
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), 12)
	if err != nil {
		http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
		return
	}

	client, err := h.clientRepo.Create(r.Context(), req.ClientID, string(hash), req.Name, req.RedirectURIs, req.AllowedScopes)
	if err != nil {
		http.Error(w, `{"error":"client_id_taken"}`, http.StatusConflict)
		return
	}

	resp := map[string]any{
		"id":             client.ID,
		"client_id":      client.ClientID,
		"client_secret":  secret,
		"name":           client.Name,
		"redirect_uris":  client.RedirectURIs,
		"allowed_scopes": client.AllowedScopes,
		"created_at":     client.CreatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// DeleteClient deletes an OIDC client by UUID (admin only)
func (h *Handler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	err := h.clientRepo.Delete(r.Context(), id)
	if err != nil {
		if err == ErrClientNotFound {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type UpdateClientRequest struct {
	Name          string   `json:"name"`
	RedirectURIs  []string `json:"redirect_uris"`
	AllowedScopes []string `json:"allowed_scopes"`
}

// UpdateClient updates mutable fields of an OIDC client by UUID (admin only)
func (h *Handler) UpdateClient(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	var req UpdateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	client, err := h.clientRepo.Update(r.Context(), id, req.Name, req.RedirectURIs, req.AllowedScopes)
	if err != nil {
		if err == ErrClientNotFound {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(client)
}

// RotateClientSecret generates a new client secret and returns it once (admin only).
func (h *Handler) RotateClientSecret(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}
	secret, err := h.clientRepo.RotateSecret(r.Context(), id)
	if err != nil {
		if err == ErrClientNotFound {
			http.Error(w, `{"error":"not_found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal_error"}`, http.StatusInternalServerError)
		return
	}

	if actor := authmw.ClaimsFrom(r.Context()); actor != nil {
		h.logAudit(r.Context(), audit.Entry{
			UserID:    &actor.UserID,
			Action:    "oidc.client_secret_rotated",
			Resource:  "oauth2_client",
			IPAddress: r.RemoteAddr,
			UserAgent: r.Header.Get("User-Agent"),
			Metadata:  map[string]any{"client_uuid": id},
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"client_secret": secret})
}

// Introspect implements RFC 7662 token introspection. The requesting client
// authenticates via HTTP Basic or form params. Active tokens return their
// claims; invalid or revoked tokens return {"active":false}.
func (h *Handler) Introspect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	if clientID == "" {
		if id, secret, ok := r.BasicAuth(); ok {
			clientID, clientSecret = id, secret
		}
	}
	if clientID == "" {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}
	if _, err := h.clientRepo.AuthenticateClient(r.Context(), clientID, clientSecret); err != nil {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	claims, err := auth.ValidateAccessToken(h.ks, r.FormValue("token"))
	if err != nil || (h.authSvc != nil && h.authSvc.IsRevoked(r.Context(), claims.ID)) {
		h.logAudit(r.Context(), audit.Entry{
			Action:    "oidc.token_introspected",
			Resource:  "oauth2",
			IPAddress: r.RemoteAddr,
			UserAgent: r.Header.Get("User-Agent"),
			Metadata:  map[string]any{"client_id": clientID, "active": false},
		})
		json.NewEncoder(w).Encode(map[string]any{"active": false})
		return
	}

	h.logAudit(r.Context(), audit.Entry{
		UserID:    &claims.UserID,
		Action:    "oidc.token_introspected",
		Resource:  "oauth2",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  map[string]any{"client_id": clientID, "active": true, "jti": claims.ID},
	})

	json.NewEncoder(w).Encode(map[string]any{
		"active":     true,
		"sub":        claims.UserID,
		"email":      claims.Email,
		"roles":      claims.Roles,
		"exp":        claims.ExpiresAt.Unix(),
		"iat":        claims.IssuedAt.Unix(),
		"iss":        claims.Issuer,
		"jti":        claims.ID,
		"token_type": "Bearer",
	})
}

// EndSession implements RP-Initiated Logout (OpenID Connect Session Management §5).
// Accepts GET and POST. Revokes the user's current tokens, clears session cookies,
// and redirects to post_logout_redirect_uri (validated against the client identified
// by id_token_hint) or to the application login page.
func (h *Handler) EndSession(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	idTokenHint := r.FormValue("id_token_hint")
	postLogoutRedirectURI := r.FormValue("post_logout_redirect_uri")
	state := r.FormValue("state")

	// Revoke current session tokens before clearing cookies.
	rt := ""
	at := ""
	if c, err := r.Cookie("refresh_token"); err == nil {
		rt = c.Value
	}
	if c, err := r.Cookie("access_token"); err == nil {
		at = c.Value
	}
	if h.authSvc != nil && (rt != "" || at != "") {
		h.authSvc.Logout(r.Context(), rt, at)
	}

	// Clear the four session cookies.
	for _, name := range []string{"access_token", "refresh_token", "csrf_token", "at_exp"} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1})
	}

	// Identify the user for audit purposes: prefer the live access token, fall
	// back to the id_token_hint (which may be expired — that is allowed by spec).
	var userID *string
	if claims, err := auth.ValidateAccessToken(h.ks, at); err == nil {
		userID = &claims.UserID
	} else if sub := h.subFromIDTokenHint(idTokenHint); sub != "" {
		userID = &sub
	}

	h.logAudit(r.Context(), audit.Entry{
		UserID:    userID,
		Action:    "oidc.end_session",
		Resource:  "oauth2",
		IPAddress: r.RemoteAddr,
		UserAgent: r.Header.Get("User-Agent"),
	})

	// post_logout_redirect_uri is only honoured when accompanied by a valid
	// id_token_hint whose aud identifies a registered client that has the URI
	// in its redirect_uris list (reusing the existing per-client URI allowlist).
	redirectTarget := h.publicAppURL + "/auth/login"
	if postLogoutRedirectURI != "" && idTokenHint != "" {
		if clientID := h.clientIDFromIDTokenHint(idTokenHint); clientID != "" {
			if client, err := h.clientRepo.FindByClientID(r.Context(), clientID); err == nil {
				if client.ValidateRedirectURI(postLogoutRedirectURI) {
					parsed, err := url.Parse(postLogoutRedirectURI)
					if err == nil {
						if state != "" {
							q := parsed.Query()
							q.Set("state", state)
							parsed.RawQuery = q.Encode()
						}
						redirectTarget = parsed.String()
					}
				}
			}
		}
	}

	http.Redirect(w, r, redirectTarget, http.StatusFound)
}

// parseIDTokenHint validates the signature and structure of an ID token hint.
// Per OIDC Session Management §5, an expired token is still accepted as a hint.
func (h *Handler) parseIDTokenHint(hint string) (jwt.MapClaims, error) {
	if h.ks == nil || hint == "" {
		return nil, errors.New("no keystore or empty hint")
	}
	keyfunc := func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, errors.New("unexpected signing method")
		}
		kid, _ := t.Header["kid"].(string)
		pub, exists := h.ks.PublicKey(kid)
		if !exists {
			return nil, errors.New("unknown kid")
		}
		return pub, nil
	}
	token, err := jwt.ParseWithClaims(hint, jwt.MapClaims{}, keyfunc)
	// Accept expired tokens — they are valid hints per the spec.
	if err != nil && !errors.Is(err, jwt.ErrTokenExpired) {
		return nil, err
	}
	if token == nil {
		return nil, errors.New("nil token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("unexpected claims type")
	}
	return claims, nil
}

func (h *Handler) subFromIDTokenHint(hint string) string {
	claims, err := h.parseIDTokenHint(hint)
	if err != nil {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

func (h *Handler) clientIDFromIDTokenHint(hint string) string {
	claims, err := h.parseIDTokenHint(hint)
	if err != nil {
		return ""
	}
	switch v := claims["aud"].(type) {
	case string:
		return v
	case []any:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}
	return ""
}


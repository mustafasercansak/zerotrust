package oidc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/internal/user"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	svc          *Service
	clientRepo   *ClientRepository
	userSvc      *user.Service
	authSvc      *auth.Service
	ks           *auth.KeyStore
	issuer       string
	publicAppURL string
}

func NewHandler(svc *Service, clientRepo *ClientRepository, userSvc *user.Service, authSvc *auth.Service, ks *auth.KeyStore, issuer string, publicAppURL string) *Handler {
	return &Handler{
		svc:          svc,
		clientRepo:   clientRepo,
		userSvc:      userSvc,
		authSvc:      authSvc,
		ks:           ks,
		issuer:       issuer,
		publicAppURL: publicAppURL,
	}
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

	// Manually authenticate user session using the access_token cookie
	var claims *auth.Claims
	if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
		validated, vErr := auth.ValidateAccessToken(h.ks, c.Value)
		if vErr == nil && !h.authSvc.IsRevoked(r.Context(), validated.ID) {
			claims = validated
		}
	}

	if claims == nil || claims.UserID == "" {
		// User is not logged in. Redirect to the UI login page, passing this authorize URL as the return path.
		loginURL := h.publicAppURL + "/auth/login?redirect_to=" + url.QueryEscape(r.RequestURI)
		http.Redirect(w, r, loginURL, http.StatusFound)
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
		if vErr == nil && !h.authSvc.IsRevoked(r.Context(), validated.ID) {
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

	if !req.Approved {
		// Redirect back with access_denied
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
	client, err := h.clientRepo.FindByClientID(r.Context(), req.ClientID)
	if err != nil {
		http.Error(w, `{"error":"invalid_client"}`, http.StatusUnauthorized)
		return
	}
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
		http.Error(w, `{"error":"server_error","error_description":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

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
	if grantType != "authorization_code" {
		http.Error(w, `{"error":"unsupported_grant_type"}`, http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")

	// Read from Authorization header basic auth if client ID / Secret are not in the form
	if clientID == "" {
		username, password, ok := r.BasicAuth()
		if ok {
			clientID = username
			clientSecret = password
		}
	}

	if code == "" || clientID == "" || redirectURI == "" {
		http.Error(w, `{"error":"invalid_request","error_description":"code, client_id, and redirect_uri are required"}`, http.StatusBadRequest)
		return
	}

	resp, err := h.svc.ExchangeCode(r.Context(), code, clientID, clientSecret, redirectURI, codeVerifier)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(resp)
}

// UserInfo returns OIDC UserInfo claims
func (h *Handler) UserInfo(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

	claims, err := auth.ValidateAccessToken(h.ks, tokenStr)
	if err != nil || h.authSvc.IsRevoked(r.Context(), claims.ID) {
		http.Error(w, `{"error":"invalid_token"}`, http.StatusUnauthorized)
		return
	}

	u, err := h.userSvc.FindByID(r.Context(), claims.UserID)
	if err != nil {
		http.Error(w, `{"error":"invalid_token"}`, http.StatusUnauthorized)
		return
	}

	resp := map[string]any{
		"sub":            u.ID,
		"name":           strings.TrimSpace(u.FirstName + " " + u.LastName),
		"given_name":     u.FirstName,
		"family_name":    u.LastName,
		"email":          u.Email,
		"email_verified": true,
		"locale":         u.Locale,
		"roles":          u.Roles,
		"groups":         u.Roles,
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
		"scopes_supported":                      []string{"openid", "profile", "email"},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported":  []string{"EdDSA"},
		"code_challenge_methods_supported":        []string{"S256"},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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


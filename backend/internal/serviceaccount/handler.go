package serviceaccount

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/zerotrust/backend/internal/auth"
	"github.com/zerotrust/backend/pkg/middleware"
)

type Handler struct {
	svc     *Service
	hub     *EventHub
	ks      *auth.KeyStore
	authSvc *auth.Service
}

func NewHandler(svc *Service, hub *EventHub, ks *auth.KeyStore, authSvc *auth.Service) *Handler {
	return &Handler{svc: svc, hub: hub, ks: ks, authSvc: authSvc}
}

type saResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	ClientID  string  `json:"client_id"`
	IsActive  bool    `json:"is_active"`
	Scopes    []string `json:"scopes"`
	CreatedAt string  `json:"created_at"`
	ExpiresAt *string `json:"expires_at"`
}

// createResponse is returned only on creation — includes the plaintext secret shown once.
type createResponse struct {
	saResponse
	ClientSecret string `json:"client_secret"`
}

func toResponse(sa *ServiceAccount) saResponse {
	scopes := sa.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	var expiresAt *string
	if sa.ExpiresAt != nil {
		s := sa.ExpiresAt.Format("2006-01-02T15:04:05Z")
		expiresAt = &s
	}
	return saResponse{
		ID:        sa.ID,
		Name:      sa.Name,
		ClientID:  sa.ClientID,
		IsActive:  sa.IsActive,
		Scopes:    scopes,
		CreatedAt: sa.CreatedAt.Format("2006-01-02T15:04:05Z"),
		ExpiresAt: expiresAt,
	}
}

type pagedSAResponse struct {
	Data  []saResponse `json:"data"`
	Total int          `json:"total"`
}

// GET /api/v1/admin/service-accounts?limit=25&offset=0&sort_by=name&sort_dir=asc&name=foo&status=active
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result, err := h.svc.List(r.Context(), ListParams{
		Limit:   queryInt(q.Get("limit"), 25),
		Offset:  queryInt(q.Get("offset"), 0),
		SortBy:  q.Get("sort_by"),
		SortDir: q.Get("sort_dir"),
		Name:    q.Get("name"),
		Status:  q.Get("status"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	data := make([]saResponse, len(result.Accounts))
	for i, sa := range result.Accounts {
		data[i] = toResponse(sa)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pagedSAResponse{Data: data, Total: result.Total})
}

type createRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at"`
}

// POST /api/v1/admin/service-accounts
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02", *req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_expires_at")
			return
		}
		// Set to end of day UTC so the full selected date is valid
		eod := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, time.UTC)
		expiresAt = &eod
	}

	caller := middleware.ClaimsFrom(r.Context())
	sa, secret, err := h.svc.Create(r.Context(), req.Name, "", caller, req.Scopes, expiresAt)
	if err != nil {
		switch {
		case errors.Is(err, ErrNameTaken):
			writeError(w, http.StatusConflict, "name_taken")
		case errors.Is(err, ErrUnknownScope):
			writeError(w, http.StatusUnprocessableEntity, "unknown_scope")
		case errors.Is(err, ErrForbiddenScope):
			writeError(w, http.StatusForbidden, "forbidden_scope")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createResponse{
		saResponse:   toResponse(sa),
		ClientSecret: secret,
	})
}

// DELETE /api/v1/admin/service-accounts/{id} — permanently removes the account
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.Revoke(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PATCH /api/v1/admin/service-accounts/{id}/status — toggles is_active
func (h *Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		IsActive bool `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := h.svc.SetActive(r.Context(), id, req.IsActive); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/admin/service-accounts/events — SSE stream.
// Auth: httpOnly cookie (same-origin EventSource sends it automatically) with ?token= fallback.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	token := ""
	if c, err := r.Cookie("access_token"); err == nil {
		token = c.Value
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		writeError(w, http.StatusUnauthorized, "missing_token")
		return
	}
	claims, err := auth.ValidateAccessToken(h.ks, token)
	if err != nil || !claims.HasPermission("service_accounts", "read") {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	fmt.Fprintf(w, "data: connected\n\n")
	flusher.Flush()

	ch, unsub := h.hub.Subscribe()
	defer unsub()

	// Bound the stream lifetime to the token's expiry.
	ctx, cancel := context.WithDeadline(r.Context(), claims.ExpiresAt.Time)
	defer cancel()

	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if h.authSvc.IsRevoked(ctx, claims.ID) {
				fmt.Fprintf(w, "data: revoked\n\n")
				flusher.Flush()
				return
			}
		case <-ch:
			fmt.Fprintf(w, "data: change\n\n")
			flusher.Flush()
		}
	}
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func queryInt(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n
	}
	return def
}

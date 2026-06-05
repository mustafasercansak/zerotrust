package webauthn

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	authmw "github.com/zerotrust/backend/pkg/middleware"
)

// service is the subset of *Service used by registration/management endpoints.
type service interface {
	BeginRegistration(ctx context.Context, userID, name, displayName string) (json.RawMessage, error)
	FinishRegistration(ctx context.Context, userID, name, displayName, credName string, responseBody []byte) error
	ListCredentials(ctx context.Context, userID string) ([]CredentialMeta, error)
	DeleteCredential(ctx context.Context, id, userID string) error
}

type Handler struct {
	svc service
}

func NewHandler(svc service) *Handler {
	return &Handler{svc: svc}
}

// POST /api/v1/webauthn/register/begin — start passkey registration.
func (h *Handler) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	options, err := h.svc.BeginRegistration(r.Context(), claims.UserID, claims.Email, claims.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(options)
}

// POST /api/v1/webauthn/register/finish — verify attestation and store the passkey.
// Body: {"name":"My YubiKey","credential":{...attestation response...}}
func (h *Handler) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Name       string          `json:"name"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Credential) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Passkey"
	}
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}

	err := h.svc.FinishRegistration(r.Context(), claims.UserID, claims.Email, claims.Email, name, req.Credential)
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	case errors.Is(err, ErrCredentialInUse):
		writeError(w, http.StatusConflict, "credential_already_registered")
	case errors.Is(err, ErrSessionNotFound):
		writeError(w, http.StatusBadRequest, "ceremony_expired")
	default:
		writeError(w, http.StatusBadRequest, "invalid_credential")
	}
}

// GET /api/v1/webauthn/credentials — list the user's registered passkeys.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	creds, err := h.svc.ListCredentials(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"credentials": creds})
}

// DELETE /api/v1/webauthn/credentials/{id} — remove one of the user's passkeys.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	err := h.svc.DeleteCredential(r.Context(), id, claims.UserID)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error")
	}
}

func writeError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

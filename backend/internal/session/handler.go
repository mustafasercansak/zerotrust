package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	authmw "github.com/zerotrust/backend/pkg/middleware"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// GET /api/v1/sessions — list the caller's active sessions
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeSessionError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	currentHash := ""
	if c, err := r.Cookie("refresh_token"); err == nil && c.Value != "" {
		currentHash = hashRefreshToken(c.Value)
	}

	sessions, err := h.repo.ListForUser(r.Context(), claims.UserID, currentHash)
	if err != nil {
		writeSessionError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// DELETE /api/v1/sessions/{id} — revoke one of the caller's sessions
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeSessionError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeSessionError(w, http.StatusBadRequest, "invalid_request")
		return
	}

	if err := h.repo.RevokeByID(r.Context(), id, claims.UserID); err != nil {
		if err == ErrNotFound {
			writeSessionError(w, http.StatusNotFound, "not_found")
			return
		}
		writeSessionError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func hashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func writeSessionError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

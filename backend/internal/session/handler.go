package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	authmw "github.com/zerotrust/backend/pkg/middleware"
)

type store interface {
	ListForUser(ctx context.Context, userID, currentHash string) ([]SessionInfo, error)
	RevokeByID(ctx context.Context, id, userID string) error
	RevokeOtherSessions(ctx context.Context, userID, currentHash string) error
}

// revocationChecker reports whether an access token's JTI has been
// blocklisted (e.g. by logout). *auth.Service satisfies this.
type revocationChecker interface {
	IsRevoked(ctx context.Context, jti string) bool
}

const defaultEventTickInterval = 15 * time.Second

type Handler struct {
	repo         store
	hub          *EventHub
	revoked      revocationChecker // optional; nil disables the per-tick revocation check
	tickInterval time.Duration     // overridden in tests; production always uses the default
}

func NewHandler(repo store, hub *EventHub) *Handler {
	return &Handler{repo: repo, hub: hub, tickInterval: defaultEventTickInterval}
}

// SetRevocationChecker wires JTI-blocklist checks into the SSE stream so a
// token revoked (e.g. by logout) mid-stream ends the connection instead of
// leaving it open until the next session-table event. (ISSUE_LIST #99)
func (h *Handler) SetRevocationChecker(rc revocationChecker) {
	h.revoked = rc
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

// GET /api/v1/sessions/events — per-user SSE stream for live session updates.
func (h *Handler) Events(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeSessionError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	currentHash := ""
	if c, err := r.Cookie("refresh_token"); err == nil && c.Value != "" {
		currentHash = hashRefreshToken(c.Value)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeSessionError(w, http.StatusInternalServerError, "streaming_unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	fmt.Fprintf(w, "data: connected\n\n")
	flusher.Flush()

	ch, unsub := h.hub.Subscribe(claims.UserID)
	defer unsub()

	var tokenExpiry time.Time
	if claims.ExpiresAt != nil {
		tokenExpiry = claims.ExpiresAt.Time
	}
	jti := claims.ID

	tick := time.NewTicker(h.tickInterval)
	defer tick.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			// The access token that authenticated this connection is
			// re-checked on every tick, so a stolen token's live feed of
			// session activity doesn't outlive the token's validity or
			// survive a logout. (ISSUE_LIST #99)
			if !tokenExpiry.IsZero() && !time.Now().Before(tokenExpiry) {
				fmt.Fprintf(w, "data: expired\n\n")
				flusher.Flush()
				return
			}
			if jti != "" && h.revoked != nil && h.revoked.IsRevoked(r.Context(), jti) {
				fmt.Fprintf(w, "data: revoked\n\n")
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case event := <-ch:
			switch event.Kind {
			case "revoked_all":
				// All sessions wiped (token reuse, admin action) — this one too.
				fmt.Fprintf(w, "data: revoked\n\n")
				flusher.Flush()
				return
			case "revoked_others":
				if event.SessionHash == currentHash {
					// This is the session that triggered "revoke others" — it's kept.
					fmt.Fprintf(w, "data: change\n\n")
					flusher.Flush()
				} else {
					// This session was among the revoked ones.
					fmt.Fprintf(w, "data: revoked\n\n")
					flusher.Flush()
					return
				}
			case "revoked":
				if event.SessionHash == currentHash {
					fmt.Fprintf(w, "data: revoked\n\n")
					flusher.Flush()
					return
				}
				fmt.Fprintf(w, "data: change\n\n")
				flusher.Flush()
			default:
				fmt.Fprintf(w, "data: change\n\n")
				flusher.Flush()
			}
		}
	}
}

// DELETE /api/v1/sessions — revoke every session except the caller's current one.
func (h *Handler) RevokeOthers(w http.ResponseWriter, r *http.Request) {
	claims := authmw.ClaimsFrom(r.Context())
	if claims == nil {
		writeSessionError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	c, err := r.Cookie("refresh_token")
	if err != nil || c.Value == "" {
		writeSessionError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	currentHash := hashRefreshToken(c.Value)

	if err := h.repo.RevokeOtherSessions(r.Context(), claims.UserID, currentHash); err != nil {
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

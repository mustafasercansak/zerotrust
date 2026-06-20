package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/zerotrust/backend/internal/session"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/internal/webauthn"
	"github.com/zerotrust/backend/pkg/middleware"
	"github.com/zerotrust/backend/pkg/validation"
)

// SessionManager is the subset of session.Repository that admin endpoints need.
// *session.Repository satisfies this interface directly.
type SessionManager interface {
	ListForUser(ctx context.Context, userID, currentHash string) ([]session.SessionInfo, error)
	RevokeAllForUser(ctx context.Context, userID string) error
	RevokeByID(ctx context.Context, id, userID string) error
}

// UserManager is the subset of user.Service used by admin endpoints.
// *user.Service satisfies this interface directly.
type UserManager interface {
	List(ctx context.Context, p user.ListParams) (user.ListResult, error)
	RegisterWithRoles(ctx context.Context, email, password, locale string, roles []string) (*user.User, error)
	UpdateProfile(ctx context.Context, userID, firstName, lastName string) (*user.User, error)
	SetRoles(ctx context.Context, userID string, roles []string) error
	SetActive(ctx context.Context, userID string, active bool) error
	FindByID(ctx context.Context, id string) (*user.User, error)
}

type MfaRepo interface {
	IsEnabledForUser(ctx context.Context, userID string) (bool, error)
	EnabledForUsers(ctx context.Context, userIDs []string) (map[string]bool, error)
}

type WebAuthnRepo interface {
	ListMeta(ctx context.Context, userID string) ([]webauthn.CredentialMeta, error)
	CountByUsers(ctx context.Context, userIDs []string) (map[string]int, error)
}

type SecurityPostureProvider interface {
	SecurityPosture(ctx context.Context) (user.SecurityPostureStats, error)
}

type Handler struct {
	userSvc  UserManager
	sessions SessionManager // nil when not wired
	webauthn WebAuthnRepo
	mfa      MfaRepo
	posture  SecurityPostureProvider
}

func NewHandler(userSvc UserManager, sessions SessionManager, webauthn WebAuthnRepo, mfa MfaRepo) *Handler {
	return &Handler{
		userSvc:  userSvc,
		sessions: sessions,
		webauthn: webauthn,
		mfa:      mfa,
	}
}

func (h *Handler) SetPostureProvider(p SecurityPostureProvider) {
	h.posture = p
}

type userResponse struct {
	ID             string   `json:"id"`
	Email          string   `json:"email"`
	FirstName      string   `json:"first_name"`
	LastName       string   `json:"last_name"`
	HasAvatar      bool     `json:"has_avatar"`
	Locale         string   `json:"locale"`
	IsActive       bool     `json:"is_active"`
	Roles          []string `json:"roles"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	ActiveSessions int      `json:"active_sessions"`
	MfaEnabled     bool     `json:"mfa_enabled"`
	PasskeyCount   int      `json:"passkey_count"`
}

type pagedResponse[T any] struct {
	Data  []T `json:"data"`
	Total int `json:"total"`
}

func toResponse(u *user.User, activeSessions int, mfaEnabled bool, passkeyCount int) userResponse {
	roles := u.Roles
	if roles == nil {
		roles = []string{}
	}
	return userResponse{
		ID:             u.ID,
		Email:          u.Email,
		FirstName:      u.FirstName,
		LastName:       u.LastName,
		HasAvatar:      u.HasAvatar,
		Locale:         u.Locale,
		IsActive:       u.IsActive,
		Roles:          roles,
		CreatedAt:      u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:      u.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		ActiveSessions: activeSessions,
		MfaEnabled:     mfaEnabled,
		PasskeyCount:   passkeyCount,
	}
}

// GET /api/v1/admin/users
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result, err := h.userSvc.List(r.Context(), user.ListParams{
		Limit:   queryInt(q.Get("limit"), 25),
		Offset:  queryInt(q.Get("offset"), 0),
		SortBy:  q.Get("sort_by"),
		SortDir: q.Get("sort_dir"),
		Email:   q.Get("email"),
		Status:  q.Get("status"),
		Role:    q.Get("role"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	// Collect user IDs for batch security-posture lookups.
	userIDs := make([]string, len(result.Users))
	for i, u := range result.Users {
		userIDs[i] = u.ID
	}

	var mfaEnabled map[string]bool
	if h.mfa != nil {
		mfaEnabled, _ = h.mfa.EnabledForUsers(r.Context(), userIDs)
	}
	var passkeyCounts map[string]int
	if h.webauthn != nil {
		passkeyCounts, _ = h.webauthn.CountByUsers(r.Context(), userIDs)
	}

	data := make([]userResponse, len(result.Users))
	for i, u := range result.Users {
		sessionCount := 0
		if result.ActiveSessions != nil {
			sessionCount = result.ActiveSessions[u.ID]
		}
		data[i] = toResponse(u, sessionCount, mfaEnabled[u.ID], passkeyCounts[u.ID])
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pagedResponse[userResponse]{Data: data, Total: result.Total})
}

type createUserRequest struct {
	Email     string   `json:"email"`
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Password  string   `json:"password"`
	Locale    string   `json:"locale"`
	Roles     []string `json:"roles"`
}

// POST /api/v1/admin/users
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
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
	if len([]rune(strings.TrimSpace(req.FirstName))) > 80 || len([]rune(strings.TrimSpace(req.LastName))) > 80 {
		writeError(w, http.StatusBadRequest, "invalid_profile")
		return
	}

	u, err := h.userSvc.RegisterWithRoles(r.Context(), req.Email, req.Password, req.Locale, req.Roles)
	if err != nil {
		switch {
		case errors.Is(err, user.ErrEmailTaken):
			writeError(w, http.StatusConflict, "email_taken")
		case errors.Is(err, user.ErrUnknownRole):
			writeError(w, http.StatusUnprocessableEntity, "unknown_role")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}
	if req.FirstName != "" || req.LastName != "" {
		u, err = h.userSvc.UpdateProfile(r.Context(), u.ID, req.FirstName, req.LastName)
		if err != nil {
			if errors.Is(err, user.ErrInvalidProfile) {
				writeError(w, http.StatusBadRequest, "invalid_profile")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toResponse(u, 0, false, 0))
}

type updateRolesRequest struct {
	Roles []string `json:"roles"`
}

// PATCH /api/v1/admin/users/{id}/roles
func (h *Handler) UpdateRoles(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	// Admins may not change their own roles — prevents self-demotion lockout and
	// self-escalation. (ISSUE_LIST #34)
	if claims := middleware.ClaimsFrom(r.Context()); claims != nil && claims.UserID == userID {
		writeError(w, http.StatusForbidden, "self_modification_forbidden")
		return
	}
	var req updateRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := h.userSvc.SetRoles(r.Context(), userID, req.Roles); err != nil {
		switch {
		case errors.Is(err, user.ErrUnknownRole):
			writeError(w, http.StatusUnprocessableEntity, "unknown_role")
		case errors.Is(err, user.ErrLastAdmin):
			writeError(w, http.StatusConflict, "last_admin")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type setStatusRequest struct {
	IsActive bool `json:"is_active"`
}

// PATCH /api/v1/admin/users/{id}/status
// Activates or deactivates a user. Deactivation also revokes all their sessions.
func (h *Handler) SetStatus(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	// Admins may not change their own active status — prevents self-lockout.
	// (ISSUE_LIST #34)
	if claims := middleware.ClaimsFrom(r.Context()); claims != nil && claims.UserID == userID {
		writeError(w, http.StatusForbidden, "self_modification_forbidden")
		return
	}
	var req setStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := h.userSvc.SetActive(r.Context(), userID, req.IsActive); err != nil {
		switch {
		case errors.Is(err, user.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found")
		case errors.Is(err, user.ErrLastAdmin):
			writeError(w, http.StatusConflict, "last_admin")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}
	// Deactivating a user must immediately revoke all active sessions.
	if !req.IsActive && h.sessions != nil {
		_ = h.sessions.RevokeAllForUser(r.Context(), userID)
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/admin/users/{id}/sessions
func (h *Handler) ListUserSessions(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		writeError(w, http.StatusServiceUnavailable, "sessions_unavailable")
		return
	}
	userID := chi.URLParam(r, "id")
	sessions, err := h.sessions.ListForUser(r.Context(), userID, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// DELETE /api/v1/admin/users/{id}/sessions — revoke ALL sessions for a user
func (h *Handler) RevokeAllUserSessions(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		writeError(w, http.StatusServiceUnavailable, "sessions_unavailable")
		return
	}
	userID := chi.URLParam(r, "id")
	if err := h.sessions.RevokeAllForUser(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/v1/admin/users/{id}/sessions/{sessionId} — revoke one session
func (h *Handler) RevokeUserSession(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		writeError(w, http.StatusServiceUnavailable, "sessions_unavailable")
		return
	}
	userID := chi.URLParam(r, "id")
	sessionID := chi.URLParam(r, "sessionId")
	if err := h.sessions.RevokeByID(r.Context(), sessionID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/admin/users/{id}/mfa
func (h *Handler) GetUserMfa(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")

	if _, err := h.userSvc.FindByID(r.Context(), userID); err != nil {
		if errors.Is(err, user.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal_error")
		}
		return
	}

	totpEnabled := false
	if h.mfa != nil {
		enabled, err := h.mfa.IsEnabledForUser(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		totpEnabled = enabled
	}

	var credentials []webauthn.CredentialMeta
	if h.webauthn != nil {
		var err error
		credentials, err = h.webauthn.ListMeta(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
	} else {
		credentials = []webauthn.CredentialMeta{}
	}

	resp := map[string]any{
		"totp_enabled":         totpEnabled,
		"webauthn_credentials": credentials,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GET /api/v1/admin/security-posture
func (h *Handler) SecurityPosture(w http.ResponseWriter, r *http.Request) {
	if h.posture == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable")
		return
	}
	stats, err := h.posture.SecurityPosture(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
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

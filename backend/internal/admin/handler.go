package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/zerotrust/backend/internal/user"
	"github.com/zerotrust/backend/pkg/validation"
)

type Handler struct {
	userSvc *user.Service
}

func NewHandler(userSvc *user.Service) *Handler {
	return &Handler{userSvc: userSvc}
}

type userResponse struct {
	ID        string   `json:"id"`
	Email     string   `json:"email"`
	Locale    string   `json:"locale"`
	IsActive  bool     `json:"is_active"`
	Roles     []string `json:"roles"`
	CreatedAt string   `json:"created_at"`
}

type pagedResponse[T any] struct {
	Data  []T `json:"data"`
	Total int `json:"total"`
}

func toResponse(u *user.User) userResponse {
	roles := u.Roles
	if roles == nil {
		roles = []string{}
	}
	return userResponse{
		ID:        u.ID,
		Email:     u.Email,
		Locale:    u.Locale,
		IsActive:  u.IsActive,
		Roles:     roles,
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// GET /api/v1/admin/users?limit=25&offset=0&sort_by=email&sort_dir=asc&email=foo&status=active
func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result, err := h.userSvc.List(r.Context(), user.ListParams{
		Limit:   queryInt(q.Get("limit"), 25),
		Offset:  queryInt(q.Get("offset"), 0),
		SortBy:  q.Get("sort_by"),
		SortDir: q.Get("sort_dir"),
		Email:   q.Get("email"),
		Status:  q.Get("status"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}

	data := make([]userResponse, len(result.Users))
	for i, u := range result.Users {
		data[i] = toResponse(u)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pagedResponse[userResponse]{Data: data, Total: result.Total})
}

type createUserRequest struct {
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Locale   string   `json:"locale"`
	Roles    []string `json:"roles"`
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toResponse(u))
}

type updateRolesRequest struct {
	Roles []string `json:"roles"`
}

// PATCH /api/v1/admin/users/{id}/roles
func (h *Handler) UpdateRoles(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	var req updateRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := h.userSvc.SetRoles(r.Context(), userID, req.Roles); err != nil {
		if errors.Is(err, user.ErrUnknownRole) {
			writeError(w, http.StatusUnprocessableEntity, "unknown_role")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

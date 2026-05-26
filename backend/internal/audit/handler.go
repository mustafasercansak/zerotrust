package audit

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

type pagedResponse struct {
	Data  []EntryRow `json:"data"`
	Total int        `json:"total"`
}

// GET /api/v1/admin/audit?limit=25&offset=0&sort_by=created_at&sort_dir=desc&action=login&user_id=...
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result, err := h.repo.List(r.Context(), ListParams{
		Limit:    queryInt(q.Get("limit"), 25),
		Offset:   queryInt(q.Get("offset"), 0),
		SortBy:   q.Get("sort_by"),
		SortDir:  q.Get("sort_dir"),
		Action:   q.Get("action"),
		UserID:   q.Get("user_id"),
		Resource: q.Get("resource"),
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal_error"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pagedResponse{Data: result.Entries, Total: result.Total})
}

func queryInt(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n
	}
	return def
}

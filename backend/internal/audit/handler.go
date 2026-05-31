package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

type AuditStore interface {
	List(ctx context.Context, p ListParams) (ListResult, error)
	Trends(ctx context.Context) ([]TrendPoint, error)
}

type Handler struct {
	repo AuditStore
}

func NewHandler(repo AuditStore) *Handler {
	return &Handler{repo: repo}
}

type pagedResponse struct {
	Data  []EntryRow `json:"data"`
	Total int        `json:"total"`
}

// GET /api/v1/admin/audit?limit=25&offset=0&sort_by=created_at&sort_dir=desc&action=login&user_id=...&outcome=success
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
		Outcome:  q.Get("outcome"),
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

// GET /api/v1/admin/audit/trends
func (h *Handler) Trends(w http.ResponseWriter, r *http.Request) {
	points, err := h.repo.Trends(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal_error"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}

func queryInt(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n
	}
	return def
}

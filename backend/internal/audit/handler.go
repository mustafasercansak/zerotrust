package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type AuditStore interface {
	List(ctx context.Context, p ListParams) (ListResult, error)
	Trends(ctx context.Context) ([]TrendPoint, error)
	SecurityDashboard(ctx context.Context, rangeValue string) (SecurityDashboard, error)
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

// GET /api/v1/admin/security-dashboard?range=24h|7d|30d
func (h *Handler) SecurityDashboard(w http.ResponseWriter, r *http.Request) {
	result, err := h.repo.SecurityDashboard(r.Context(), r.URL.Query().Get("range"))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "internal_error"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GET /api/v1/admin/audit/export?format=csv|json&action=...&user_id=...&outcome=...
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := q.Get("format")
	if format != "json" {
		format = "csv"
	}

	result, err := h.repo.List(r.Context(), ListParams{
		Limit:    10000,
		Offset:   0,
		SortBy:   "created_at",
		SortDir:  "desc",
		Action:   q.Get("action"),
		UserID:   q.Get("user_id"),
		Resource: q.Get("resource"),
		Outcome:  q.Get("outcome"),
	})
	if err != nil {
		http.Error(w, "internal_error", http.StatusInternalServerError)
		return
	}

	filename := "audit-log-" + time.Now().UTC().Format("2006-01-02") + "." + format

	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		json.NewEncoder(w).Encode(result.Entries)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	// UTF-8 BOM for Excel compatibility
	w.Write([]byte("\xef\xbb\xbf"))

	cw := csv.NewWriter(w)
	cw.Write([]string{"time", "action", "resource", "user_email", "user_id", "ip_address"})
	for _, e := range result.Entries {
		cw.Write([]string{
			e.CreatedAt,
			e.Action,
			e.Resource,
			derefStr(e.UserEmail),
			derefStr(e.UserID),
			derefStr(e.IPAddress),
		})
	}
	cw.Flush()
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func queryInt(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n >= 0 {
		return n
	}
	return def
}

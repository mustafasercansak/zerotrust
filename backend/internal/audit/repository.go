package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	UserID    *string
	Action    string
	Resource  string
	IPAddress string
	UserAgent string
	Metadata  map[string]any
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Log(ctx context.Context, e Entry) error {
	var meta []byte
	if len(e.Metadata) > 0 {
		var err error
		meta, err = json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("marshal audit metadata: %w", err)
		}
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, resource, ip_address, user_agent, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, e.UserID, e.Action, e.Resource, nullStr(e.IPAddress), nullStr(e.UserAgent), meta)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

// EntryRow is the read model for audit log listing.
type EntryRow struct {
	ID        string         `json:"id"`
	UserID    *string        `json:"user_id"`
	UserEmail *string        `json:"user_email"`
	Action    string         `json:"action"`
	Resource  string         `json:"resource"`
	IPAddress *string        `json:"ip_address"`
	UserAgent *string        `json:"user_agent"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt string         `json:"created_at"`
}

// ListParams configures pagination, sorting, and filtering for List.
type ListParams struct {
	Limit    int
	Offset   int
	SortBy   string // created_at | action | user_id | resource
	SortDir  string // asc | desc
	Action   string // ILIKE filter
	UserID   string // exact match
	Resource string // ILIKE filter
}

// ListResult holds one page of audit entries and the total matching count.
type ListResult struct {
	Entries []EntryRow
	Total   int
}

var auditSortCols = map[string]string{
	"created_at": "created_at",
	"action":     "action",
	"user_id":    "user_id",
	"resource":   "resource",
}

// List returns a filtered, sorted, paginated page of audit entries with the total count.
func (r *Repository) List(ctx context.Context, p ListParams) (ListResult, error) {
	if p.Limit <= 0 || p.Limit > 200 {
		p.Limit = 25
	}
	col, ok := auditSortCols[p.SortBy]
	if !ok {
		col = "created_at"
	}
	dir := "DESC"
	if strings.EqualFold(p.SortDir, "asc") {
		dir = "ASC"
	}

	var conds []string
	var args []any
	n := 1

	if p.Action != "" {
		conds = append(conds, fmt.Sprintf("a.action ILIKE $%d", n))
		args = append(args, "%"+p.Action+"%")
		n++
	}
	if p.UserID != "" {
		conds = append(conds, fmt.Sprintf("a.user_id::text = $%d", n))
		args = append(args, p.UserID)
		n++
	}
	if p.Resource != "" {
		conds = append(conds, fmt.Sprintf("a.resource ILIKE $%d", n))
		args = append(args, "%"+p.Resource+"%")
		n++
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM audit_logs a LEFT JOIN users u ON u.id = a.user_id %s`, where),
		args...,
	).Scan(&total); err != nil {
		return ListResult{}, err
	}

	dataArgs := append(append([]any{}, args...), p.Limit, p.Offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT a.id::text,
		       a.user_id::text,
		       u.email,
		       a.action,
		       a.resource,
		       a.ip_address::text,
		       a.user_agent,
		       COALESCE(a.metadata, '{}'::jsonb),
		       to_char(a.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.user_id
		%s
		ORDER BY a.%s %s
		LIMIT $%d OFFSET $%d
	`, where, col, dir, n, n+1), dataArgs...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	entries := make([]EntryRow, 0)
	for rows.Next() {
		var e EntryRow
		var metadata []byte
		if err := rows.Scan(&e.ID, &e.UserID, &e.UserEmail, &e.Action, &e.Resource, &e.IPAddress, &e.UserAgent, &metadata, &e.CreatedAt); err != nil {
			return ListResult{}, err
		}
		if err := json.Unmarshal(metadata, &e.Metadata); err != nil {
			e.Metadata = map[string]any{}
		}
		entries = append(entries, e)
	}
	return ListResult{Entries: entries, Total: total}, rows.Err()
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

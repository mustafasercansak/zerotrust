package audit

import (
	"context"
	"encoding/json"

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

func (r *Repository) Log(ctx context.Context, e Entry) {
	var meta []byte
	if len(e.Metadata) > 0 {
		meta, _ = json.Marshal(e.Metadata)
	}
	_, _ = r.db.Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, resource, ip_address, user_agent, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, e.UserID, e.Action, e.Resource, nullStr(e.IPAddress), nullStr(e.UserAgent), meta)
}

// EntryRow is the read model for audit log listing.
type EntryRow struct {
	ID        string  `json:"id"`
	UserID    *string `json:"user_id"`
	Action    string  `json:"action"`
	Resource  string  `json:"resource"`
	IPAddress *string `json:"ip_address"`
	UserAgent *string `json:"user_agent"`
	CreatedAt string  `json:"created_at"`
}

// List returns audit entries ordered newest-first with limit/offset pagination.
func (r *Repository) List(ctx context.Context, limit, offset int) ([]EntryRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id::text,
		       user_id::text,
		       action,
		       resource,
		       ip_address::text,
		       user_agent,
		       to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM audit_logs
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]EntryRow, 0)
	for rows.Next() {
		var e EntryRow
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.Resource, &e.IPAddress, &e.UserAgent, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

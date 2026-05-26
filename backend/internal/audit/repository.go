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

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/pkg/secrets"
)

type Entry struct {
	UserID    *string
	Action    string
	Resource  string
	IPAddress string
	UserAgent string
	Metadata  map[string]any
}

// IPLocator resolves an IP string to a country and city name.
// *geoip.Service satisfies this via a thin closure wrapper (see main.go).
type IPLocator func(ip string) (country, city string)

type Repository struct {
	db        *pgxpool.Pool
	secClient *secrets.Client
	locator   IPLocator
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) SetSecretsClient(c *secrets.Client) {
	r.secClient = c
}

func (r *Repository) SetIPLocator(fn IPLocator) {
	r.locator = fn
}

func (r *Repository) Log(ctx context.Context, e Entry) error {
	if r.locator != nil && e.IPAddress != "" {
		if country, city := r.locator(e.IPAddress); country != "" {
			if e.Metadata == nil {
				e.Metadata = map[string]any{}
			}
			e.Metadata["location"] = map[string]any{"country": country, "city": city}
		}
	}

	var meta []byte
	if len(e.Metadata) > 0 {
		var err error
		if r.secClient != nil {
			outcome := e.Metadata["outcome"]
			status := e.Metadata["status"]
			location := e.Metadata["location"]

			sensitiveMeta := make(map[string]any)
			for k, v := range e.Metadata {
				if k != "outcome" && k != "status" && k != "location" {
					sensitiveMeta[k] = v
				}
			}

			sensBytes, err := json.Marshal(sensitiveMeta)
			if err != nil {
				return fmt.Errorf("marshal sensitive metadata: %w", err)
			}

			ciphertext, err := r.secClient.EncryptData(ctx, string(sensBytes))
			if err != nil {
				return fmt.Errorf("encrypt audit metadata: %w", err)
			}

			dbMeta := map[string]any{
				"payload": ciphertext,
			}
			if outcome != nil {
				dbMeta["outcome"] = outcome
			}
			if status != nil {
				dbMeta["status"] = status
			}
			if location != nil {
				dbMeta["location"] = location
			}

			meta, err = json.Marshal(dbMeta)
			if err != nil {
				return fmt.Errorf("marshal db metadata: %w", err)
			}
		} else {
			meta, err = json.Marshal(e.Metadata)
			if err != nil {
				return fmt.Errorf("marshal audit metadata: %w", err)
			}
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
	Outcome  string // success | failure
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
	if p.Outcome != "" {
		conds = append(conds, fmt.Sprintf("a.metadata->>'outcome' = $%d", n))
		args = append(args, p.Outcome)
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
		if r.secClient != nil && e.Metadata["payload"] != nil {
			if payloadStr, ok := e.Metadata["payload"].(string); ok && payloadStr != "" {
				decryptedStr, err := r.secClient.DecryptData(ctx, payloadStr)
				if err == nil {
					var decryptedMeta map[string]any
					if err := json.Unmarshal([]byte(decryptedStr), &decryptedMeta); err == nil {
						delete(e.Metadata, "payload")
						for k, v := range decryptedMeta {
							e.Metadata[k] = v
						}
					}
				}
			}
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

type TrendPoint struct {
	Date    string `json:"date"`
	Success int    `json:"success"`
	Failure int    `json:"failure"`
}

// Trends aggregates audit logs by day over the last 7 days, splitting into success vs failure.
func (r *Repository) Trends(ctx context.Context) ([]TrendPoint, error) {
	rows, err := r.db.Query(ctx, `
		SELECT to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day,
		       COUNT(*) FILTER (WHERE metadata->>'outcome' = 'success') AS success,
		       COUNT(*) FILTER (WHERE metadata->>'outcome' = 'failure') AS failure
		FROM audit_logs
		WHERE created_at >= NOW() - INTERVAL '7 days'
		GROUP BY day
		ORDER BY day ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query audit trends: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]TrendPoint)
	for rows.Next() {
		var day string
		var success, failure int
		if err := rows.Scan(&day, &success, &failure); err != nil {
			return nil, fmt.Errorf("scan trend row: %w", err)
		}
		counts[day] = TrendPoint{Date: day, Success: success, Failure: failure}
	}

	var points []TrendPoint
	now := time.Now().UTC()
	for i := 6; i >= 0; i-- {
		dayStr := now.AddDate(0, 0, -i).Format("2006-01-02")
		if p, ok := counts[dayStr]; ok {
			points = append(points, p)
		} else {
			points = append(points, TrendPoint{Date: dayStr, Success: 0, Failure: 0})
		}
	}

	return points, nil
}

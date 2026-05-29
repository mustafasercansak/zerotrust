package serviceaccount

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var ErrNotFound = errors.New("service_account_not_found")
var ErrNameTaken = errors.New("service_account_name_taken")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, name, createdBy string, scopes []string, expiresAt *time.Time) (*ServiceAccount, string, error) {
	clientID, err := generateClientID()
	if err != nil {
		return nil, "", err
	}
	secret, err := generateSecret()
	if err != nil {
		return nil, "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), 12)
	if err != nil {
		return nil, "", err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)

	var createdByParam *string
	if createdBy != "" {
		createdByParam = &createdBy
	}

	sa := &ServiceAccount{}
	err = tx.QueryRow(ctx, `
		INSERT INTO service_accounts (name, client_id, client_secret_hash, created_by, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, client_id, client_secret_hash, is_active, created_at, expires_at, old_client_secret_hash, old_secret_expires_at
	`, name, clientID, string(hash), createdByParam, expiresAt).Scan(
		&sa.ID, &sa.Name, &sa.ClientID, &sa.ClientSecretHash, &sa.IsActive, &sa.CreatedAt, &sa.ExpiresAt, &sa.OldClientSecretHash, &sa.OldSecretExpiresAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			return nil, "", ErrNameTaken
		}
		return nil, "", err
	}

	for _, scope := range scopes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO service_account_scopes (service_account_id, scope) VALUES ($1, $2)
		`, sa.ID, scope); err != nil {
			return nil, "", err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}

	sa.Scopes = scopes
	return sa, secret, nil
}

func (r *Repository) FindByClientID(ctx context.Context, clientID string) (*ServiceAccount, error) {
	sa := &ServiceAccount{}
	err := r.db.QueryRow(ctx, `
		SELECT id, name, client_id, client_secret_hash, is_active, created_at, expires_at, old_client_secret_hash, old_secret_expires_at
		FROM service_accounts WHERE client_id = $1
	`, clientID).Scan(
		&sa.ID, &sa.Name, &sa.ClientID, &sa.ClientSecretHash, &sa.IsActive, &sa.CreatedAt, &sa.ExpiresAt, &sa.OldClientSecretHash, &sa.OldSecretExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	sa.Scopes, _ = r.getScopes(ctx, sa.ID)
	return sa, nil
}

// ListParams configures pagination, sorting, and filtering for List.
type ListParams struct {
	Limit   int
	Offset  int
	SortBy  string // name | created_at | is_active | expires_at
	SortDir string // asc | desc
	Name    string // ILIKE filter
	Status  string // active | inactive | expired | ""
}

// ListResult holds one page of service accounts and the total matching count.
type ListResult struct {
	Accounts []*ServiceAccount
	Total    int
}

var saSortCols = map[string]string{
	"name":       "sa.name",
	"created_at": "sa.created_at",
	"is_active":  "sa.is_active",
	"expires_at": "sa.expires_at",
}

// List returns a filtered, sorted, paginated page of service accounts with the total count.
func (r *Repository) List(ctx context.Context, p ListParams) (ListResult, error) {
	if p.Limit <= 0 || p.Limit > 200 {
		p.Limit = 25
	}
	col, ok := saSortCols[p.SortBy]
	if !ok {
		col = "sa.created_at"
	}
	dir := "ASC"
	if strings.EqualFold(p.SortDir, "desc") {
		dir = "DESC"
	}

	var conds []string
	var args []any
	n := 1

	if p.Name != "" {
		conds = append(conds, fmt.Sprintf("LOWER(sa.name) LIKE $%d", n))
		args = append(args, "%"+strings.ToLower(p.Name)+"%")
		n++
	}
	switch p.Status {
	case "active":
		conds = append(conds, fmt.Sprintf("sa.is_active = $%d AND (sa.expires_at IS NULL OR sa.expires_at >= NOW())", n))
		args = append(args, true)
		n++
	case "inactive":
		conds = append(conds, fmt.Sprintf("sa.is_active = $%d", n))
		args = append(args, false)
		n++
	case "expired":
		conds = append(conds, "sa.expires_at IS NOT NULL AND sa.expires_at < NOW()")
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM service_accounts sa %s`, where),
		args...,
	).Scan(&total); err != nil {
		return ListResult{}, err
	}

	dataArgs := append(append([]any{}, args...), p.Limit, p.Offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT sa.id, sa.name, sa.client_id, sa.is_active, sa.created_at, sa.expires_at,
		       COALESCE(string_agg(s.scope, ',' ORDER BY s.scope), '') AS scopes
		FROM service_accounts sa
		LEFT JOIN service_account_scopes s ON sa.id = s.service_account_id
		%s
		GROUP BY sa.id
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, where, col, dir, n, n+1), dataArgs...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	list := make([]*ServiceAccount, 0)
	for rows.Next() {
		sa := &ServiceAccount{}
		var scopesStr string
		if err := rows.Scan(&sa.ID, &sa.Name, &sa.ClientID, &sa.IsActive, &sa.CreatedAt, &sa.ExpiresAt, &scopesStr); err != nil {
			return ListResult{}, err
		}
		if scopesStr != "" {
			sa.Scopes = strings.Split(scopesStr, ",")
		} else {
			sa.Scopes = []string{}
		}
		list = append(list, sa)
	}
	return ListResult{Accounts: list, Total: total}, rows.Err()
}

func (r *Repository) Revoke(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM service_accounts WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *Repository) Update(ctx context.Context, id, name string, scopes []string, expiresAt *time.Time, active bool) (*ServiceAccount, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	sa := &ServiceAccount{}
	err = tx.QueryRow(ctx, `
		UPDATE service_accounts
		SET name = $2, is_active = $3, expires_at = $4
		WHERE id = $1
		RETURNING id, name, client_id, client_secret_hash, is_active, created_at, expires_at, old_client_secret_hash, old_secret_expires_at
	`, id, name, active, expiresAt).Scan(
		&sa.ID, &sa.Name, &sa.ClientID, &sa.ClientSecretHash, &sa.IsActive, &sa.CreatedAt, &sa.ExpiresAt, &sa.OldClientSecretHash, &sa.OldSecretExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if strings.Contains(err.Error(), "unique") {
			return nil, ErrNameTaken
		}
		return nil, err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM service_account_scopes WHERE service_account_id = $1`, id); err != nil {
		return nil, err
	}
	for _, scope := range scopes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO service_account_scopes (service_account_id, scope) VALUES ($1, $2)
		`, id, scope); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	sa.Scopes = scopes
	return sa, nil
}

func (r *Repository) SetActive(ctx context.Context, id string, active bool) error {
	tag, err := r.db.Exec(ctx, `UPDATE service_accounts SET is_active = $2 WHERE id = $1`, id, active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r *Repository) CheckSecret(hash, secret string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil
}

// allPermissions returns the set of resource:action strings from the permissions table.
func (r *Repository) allPermissions(ctx context.Context) (map[string]bool, error) {
	rows, err := r.db.Query(ctx, `SELECT CONCAT(resource, ':', action) FROM permissions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := make(map[string]bool)
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		set[p] = true
	}
	return set, rows.Err()
}

func (r *Repository) getScopes(ctx context.Context, id string) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT scope FROM service_account_scopes WHERE service_account_id = $1 ORDER BY scope`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scopes []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		scopes = append(scopes, s)
	}
	return scopes, rows.Err()
}

func generateClientID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "svc_" + hex.EncodeToString(b), nil
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (r *Repository) RotateSecret(ctx context.Context, id string) (*ServiceAccount, string, error) {
	secret, err := generateSecret()
	if err != nil {
		return nil, "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), 12)
	if err != nil {
		return nil, "", err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback(ctx)

	// Fetch current secret hash
	var currentHash string
	err = tx.QueryRow(ctx, `SELECT client_secret_hash FROM service_accounts WHERE id = $1`, id).Scan(&currentHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}

	// Update with new secret, setting old secret to expire in 1 hour
	sa := &ServiceAccount{}
	err = tx.QueryRow(ctx, `
		UPDATE service_accounts
		SET old_client_secret_hash = $2,
		    old_secret_expires_at = NOW() + INTERVAL '1 hour',
		    client_secret_hash = $3
		WHERE id = $1
		RETURNING id, name, client_id, client_secret_hash, is_active, created_at, expires_at, old_client_secret_hash, old_secret_expires_at
	`, id, currentHash, string(hash)).Scan(
		&sa.ID, &sa.Name, &sa.ClientID, &sa.ClientSecretHash, &sa.IsActive, &sa.CreatedAt, &sa.ExpiresAt, &sa.OldClientSecretHash, &sa.OldSecretExpiresAt,
	)
	if err != nil {
		return nil, "", err
	}

	// Fetch scopes to populate
	sa.Scopes, err = r.getScopes(ctx, sa.ID)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}

	return sa, secret, nil
}

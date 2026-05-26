package serviceaccount

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
		RETURNING id, name, client_id, client_secret_hash, is_active, created_at, expires_at
	`, name, clientID, string(hash), createdByParam, expiresAt).Scan(
		&sa.ID, &sa.Name, &sa.ClientID, &sa.ClientSecretHash, &sa.IsActive, &sa.CreatedAt, &sa.ExpiresAt,
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
		SELECT id, name, client_id, client_secret_hash, is_active, created_at, expires_at
		FROM service_accounts WHERE client_id = $1
	`, clientID).Scan(
		&sa.ID, &sa.Name, &sa.ClientID, &sa.ClientSecretHash, &sa.IsActive, &sa.CreatedAt, &sa.ExpiresAt,
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

func (r *Repository) ListAll(ctx context.Context) ([]*ServiceAccount, error) {
	rows, err := r.db.Query(ctx, `
		SELECT sa.id, sa.name, sa.client_id, sa.is_active, sa.created_at, sa.expires_at,
		       COALESCE(string_agg(s.scope, ',' ORDER BY s.scope), '') AS scopes
		FROM service_accounts sa
		LEFT JOIN service_account_scopes s ON sa.id = s.service_account_id
		GROUP BY sa.id
		ORDER BY sa.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*ServiceAccount
	for rows.Next() {
		sa := &ServiceAccount{}
		var scopesStr string
		if err := rows.Scan(&sa.ID, &sa.Name, &sa.ClientID, &sa.IsActive, &sa.CreatedAt, &sa.ExpiresAt, &scopesStr); err != nil {
			return nil, err
		}
		if scopesStr != "" {
			sa.Scopes = strings.Split(scopesStr, ",")
		}
		list = append(list, sa)
	}
	return list, rows.Err()
}

func (r *Repository) Revoke(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM service_accounts WHERE id = $1`, id)
	return err
}

func (r *Repository) SetActive(ctx context.Context, id string, active bool) error {
	_, err := r.db.Exec(ctx, `UPDATE service_accounts SET is_active = $2 WHERE id = $1`, id, active)
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

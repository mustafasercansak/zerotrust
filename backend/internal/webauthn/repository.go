package webauthn

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("webauthn_credential_not_found")

// CredentialMeta is the display-facing view of a stored credential.
type CredentialMeta struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	SignCount  int64      `json:"sign_count"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Insert stores a newly registered credential. data is the full go-webauthn
// Credential serialized as JSON; credentialID is its base64url-encoded raw ID.
func (r *Repository) Insert(ctx context.Context, userID, credentialID string, data []byte, signCount int64, name string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_webauthn_credentials (user_id, credential_id, data, sign_count, name)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, credentialID, data, signCount, name)
	return err
}

// ListData returns the raw credential JSON blobs for a user, for use in
// WebAuthn ceremonies (the caller unmarshals into webauthn.Credential).
func (r *Repository) ListData(ctx context.Context, userID string) ([][]byte, error) {
	rows, err := r.db.Query(ctx, `
		SELECT data FROM user_webauthn_credentials WHERE user_id = $1 ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out [][]byte
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		out = append(out, data)
	}
	return out, rows.Err()
}

// ListMeta returns the display metadata for a user's credentials.
func (r *Repository) ListMeta(ctx context.Context, userID string) ([]CredentialMeta, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, name, sign_count, created_at, last_used_at
		FROM user_webauthn_credentials
		WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]CredentialMeta, 0)
	for rows.Next() {
		var m CredentialMeta
		if err := rows.Scan(&m.ID, &m.Name, &m.SignCount, &m.CreatedAt, &m.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Count returns the number of credentials registered for a user.
func (r *Repository) Count(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM user_webauthn_credentials WHERE user_id = $1
	`, userID).Scan(&n)
	return n, err
}

// UpdateOnLogin refreshes the stored credential after a successful assertion,
// persisting the new signature counter and last-used timestamp.
func (r *Repository) UpdateOnLogin(ctx context.Context, credentialID string, data []byte, signCount int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE user_webauthn_credentials
		SET data = $2, sign_count = $3, last_used_at = NOW()
		WHERE credential_id = $1
	`, credentialID, data, signCount)
	return err
}

// Delete removes one of a user's credentials by its row id. The user_id scope
// prevents one user from deleting another's credential. Returns ErrNotFound if
// no matching row exists.
func (r *Repository) Delete(ctx context.Context, id, userID string) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM user_webauthn_credentials WHERE id = $1::uuid AND user_id = $2::uuid
	`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CredentialExists reports whether a credential ID is already registered (any user).
func (r *Repository) CredentialExists(ctx context.Context, credentialID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM user_webauthn_credentials WHERE credential_id = $1)
	`, credentialID).Scan(&exists)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return exists, err
}

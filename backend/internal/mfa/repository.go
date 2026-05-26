package mfa

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("mfa_not_found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type record struct {
	SecretEnc string
	EnabledAt *time.Time
}

func (r *Repository) find(ctx context.Context, userID string) (*record, error) {
	var rec record
	err := r.db.QueryRow(ctx, `
		SELECT totp_secret_enc, enabled_at FROM user_mfa WHERE user_id = $1
	`, userID).Scan(&rec.SecretEnc, &rec.EnabledAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &rec, nil
}

// Upsert stores (or replaces) the encrypted TOTP secret for a user.
// enabled_at is left NULL until the user verifies a code.
func (r *Repository) Upsert(ctx context.Context, userID, secretEnc string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_mfa (user_id, totp_secret_enc)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET totp_secret_enc = EXCLUDED.totp_secret_enc, enabled_at = NULL
	`, userID, secretEnc)
	return err
}

// Enable marks the MFA record as active.
func (r *Repository) Enable(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE user_mfa SET enabled_at = NOW() WHERE user_id = $1
	`, userID)
	return err
}

// Delete removes the MFA record entirely (disable MFA).
func (r *Repository) Delete(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM user_mfa WHERE user_id = $1`, userID)
	return err
}

// IsEnabled returns true if the user has completed MFA setup.
func (r *Repository) IsEnabled(ctx context.Context, userID string) bool {
	rec, err := r.find(ctx, userID)
	return err == nil && rec.EnabledAt != nil
}

// SecretEnc returns the encrypted TOTP secret, or ErrNotFound.
func (r *Repository) SecretEnc(ctx context.Context, userID string) (string, error) {
	rec, err := r.find(ctx, userID)
	if err != nil {
		return "", err
	}
	return rec.SecretEnc, nil
}

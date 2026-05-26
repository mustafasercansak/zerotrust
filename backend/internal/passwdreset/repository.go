package passwdreset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("reset_token_not_found")
	ErrExpired  = errors.New("reset_token_expired")
	ErrUsed     = errors.New("reset_token_used")
)

const tokenTTL = time.Hour

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create stores a new reset token and returns the raw (unhashed) token.
func (r *Repository) Create(ctx context.Context, userID string) (string, error) {
	raw, err := generateToken()
	if err != nil {
		return "", err
	}
	hash := hashToken(raw)
	expiresAt := time.Now().Add(tokenTTL)

	_, err = r.db.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, hash, expiresAt)
	if err != nil {
		return "", err
	}
	return raw, nil
}

type tokenRow struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// Consume atomically validates a raw token and marks it used, returning the
// owning user ID. A SELECT FOR UPDATE inside a transaction ensures that two
// concurrent requests cannot both see the token as valid.
func (r *Repository) Consume(ctx context.Context, rawToken string) (string, error) {
	hash := hashToken(rawToken)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var row tokenRow
	err = tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, expires_at, used_at
		FROM password_reset_tokens
		WHERE token_hash = $1
		FOR UPDATE
	`, hash).Scan(&row.ID, &row.UserID, &row.ExpiresAt, &row.UsedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if row.UsedAt != nil {
		return "", ErrUsed
	}
	if time.Now().After(row.ExpiresAt) {
		return "", ErrExpired
	}

	_, err = tx.Exec(ctx, `
		UPDATE password_reset_tokens SET used_at = NOW() WHERE id = $1
	`, row.ID)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return row.UserID, nil
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

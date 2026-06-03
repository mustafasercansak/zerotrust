package passwdreset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
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

// Create invalidates any existing unused reset tokens for the user, then
// inserts a fresh one — ensuring only the most recently sent link is valid.
func (r *Repository) Create(ctx context.Context, userID string) (string, error) {
	raw, err := generateToken()
	if err != nil {
		return "", err
	}
	hash := hashToken(raw)
	expiresAt := time.Now().Add(tokenTTL)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Serialize concurrent reset requests for the same user so only one active
	// token can exist. The lock is transaction-scoped and released on commit/rollback.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1, hashtext($1))`, userID); err != nil {
		return "", err
	}

	// Cancel any previous unused tokens so old links cannot be replayed.
	if _, err = tx.Exec(ctx, `
		UPDATE password_reset_tokens
		SET used_at = NOW()
		WHERE user_id = $1 AND used_at IS NULL AND expires_at > NOW()
	`, userID); err != nil {
		return "", err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, hash, expiresAt); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
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

// ConsumeAndReset atomically validates the reset token, marks it used, updates
// the user's password hash, and revokes all their sessions in a single
// transaction. bcrypt hashing must be done by the caller before this call.
// If any step fails the transaction is rolled back and the token remains valid.
func (r *Repository) ConsumeAndReset(ctx context.Context, rawToken, newPassword, newPasswordHash string) error {
	hash := hashToken(rawToken)

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
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
			return ErrNotFound
		}
		return err
	}
	if row.UsedAt != nil {
		return ErrUsed
	}
	if time.Now().After(row.ExpiresAt) {
		return ErrExpired
	}

	// Retrieve user's current password hash to check for reuse
	var currentPasswordHash string
	err = tx.QueryRow(ctx, `
		SELECT password_hash FROM users WHERE id = $1::uuid
	`, row.UserID).Scan(&currentPasswordHash)
	if err != nil {
		return err
	}

	if bcrypt.CompareHashAndPassword([]byte(currentPasswordHash), []byte(newPassword)) == nil {
		return ErrPasswordReuseForbidden
	}

	if _, err = tx.Exec(ctx, `
		UPDATE password_reset_tokens SET used_at = NOW() WHERE id = $1
	`, row.ID); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2::uuid
	`, newPasswordHash, row.UserID); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE sessions SET is_revoked = true WHERE user_id = $1::uuid AND is_revoked = false
	`, row.UserID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

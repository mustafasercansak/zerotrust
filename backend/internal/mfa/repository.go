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
	SecretEnc            string
	PendingSecretEnc     *string
	EnabledAt            *time.Time
	RecoveryCodes        []string
	PendingRecoveryCodes []string
}

func (r *Repository) find(ctx context.Context, userID string) (*record, error) {
	var rec record
	err := r.db.QueryRow(ctx, `
		SELECT totp_secret_enc, totp_pending_enc, enabled_at, recovery_codes, pending_recovery_codes FROM user_mfa WHERE user_id = $1
	`, userID).Scan(&rec.SecretEnc, &rec.PendingSecretEnc, &rec.EnabledAt, &rec.RecoveryCodes, &rec.PendingRecoveryCodes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &rec, nil
}

// UpsertPending stores an encrypted TOTP secret as a candidate for a new setup.
// It never touches totp_secret_enc or enabled_at, so an already-enabled MFA
// row is left fully intact until the user successfully verifies the pending code.
func (r *Repository) UpsertPending(ctx context.Context, userID, pendingEnc string, pendingCodes []string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_mfa (user_id, totp_secret_enc, totp_pending_enc, pending_recovery_codes)
		VALUES ($1, '', $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET totp_pending_enc = EXCLUDED.totp_pending_enc, pending_recovery_codes = EXCLUDED.pending_recovery_codes
	`, userID, pendingEnc, pendingCodes)
	return err
}

// Enable promotes the pending secret to the active secret and sets enabled_at.
// Returns ErrNotFound if there is no pending secret to promote.
func (r *Repository) Enable(ctx context.Context, userID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE user_mfa
		SET totp_secret_enc  = totp_pending_enc,
		    totp_pending_enc = NULL,
		    recovery_codes   = pending_recovery_codes,
		    pending_recovery_codes = NULL,
		    enabled_at       = NOW()
		WHERE user_id = $1 AND totp_pending_enc IS NOT NULL
	`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateRecoveryCodes(ctx context.Context, userID string, codes []string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE user_mfa
		SET recovery_codes = $2
		WHERE user_id = $1
	`, userID, codes)
	return err
}

// ConsumeRecoveryCode atomically validates-and-removes a recovery code. The
// user's MFA row is locked (SELECT ... FOR UPDATE) while match inspects the
// stored hashes and the matching entry is removed, so two concurrent requests
// presenting the same code cannot both succeed (#95). It returns (false, nil)
// when match finds nothing, and (true, nil) once the matched code is removed.
func (r *Repository) ConsumeRecoveryCode(ctx context.Context, userID string, match func(hashes []string) (int, bool)) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var codes []string
	err = tx.QueryRow(ctx, `
		SELECT recovery_codes FROM user_mfa WHERE user_id = $1 FOR UPDATE
	`, userID).Scan(&codes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}

	idx, ok := match(codes)
	if !ok {
		return false, nil
	}

	updated := append(codes[:idx:idx], codes[idx+1:]...)
	if _, err := tx.Exec(ctx, `
		UPDATE user_mfa SET recovery_codes = $2 WHERE user_id = $1
	`, userID, updated); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// Delete removes the MFA record entirely (disables MFA).
func (r *Repository) Delete(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM user_mfa WHERE user_id = $1`, userID)
	return err
}

// IsEnabled returns true if the user has completed MFA setup.
func (r *Repository) IsEnabled(ctx context.Context, userID string) bool {
	rec, err := r.find(ctx, userID)
	return err == nil && rec.EnabledAt != nil
}

// IsEnabledForUser is like IsEnabled but surfaces unexpected DB errors.
// ErrNotFound (no MFA row) is treated as disabled, not an error.
func (r *Repository) IsEnabledForUser(ctx context.Context, userID string) (bool, error) {
	if r == nil {
		return false, nil
	}
	rec, err := r.find(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return rec.EnabledAt != nil, nil
}

// EnabledForUsers returns a set of user IDs (from the given slice) that have TOTP enabled.
// A nil receiver (MFA disabled at startup) yields an empty set instead of panicking.
func (r *Repository) EnabledForUsers(ctx context.Context, userIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(userIDs))
	if r == nil || len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT user_id FROM user_mfa
		WHERE user_id = ANY($1) AND enabled_at IS NOT NULL
	`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// SecretEnc returns the active encrypted TOTP secret, or ErrNotFound.
func (r *Repository) SecretEnc(ctx context.Context, userID string) (string, error) {
	rec, err := r.find(ctx, userID)
	if err != nil {
		return "", err
	}
	if rec.SecretEnc == "" {
		return "", ErrNotFound
	}
	return rec.SecretEnc, nil
}

// PendingSecretEnc returns the pending encrypted TOTP secret, or ErrNotFound.
func (r *Repository) PendingSecretEnc(ctx context.Context, userID string) (string, error) {
	rec, err := r.find(ctx, userID)
	if err != nil {
		return "", err
	}
	if rec.PendingSecretEnc == nil {
		return "", ErrNotFound
	}
	return *rec.PendingSecretEnc, nil
}

func (r *Repository) RecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	rec, err := r.find(ctx, userID)
	if err != nil {
		return nil, err
	}
	return rec.RecoveryCodes, nil
}

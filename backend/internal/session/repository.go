package session

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("session_not_found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create inserts a new session. ip is the client address (host:port or bare host).
func (r *Repository) Create(ctx context.Context, userID, tokenHash, ip, userAgent string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, ip_address, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, tokenHash, parseAddr(ip), userAgent, expiresAt)
	return err
}

// FindUserIDByHash returns the user ID associated with the token hash if the
// session is valid (not revoked, not expired). Returns ErrNotFound for a
// missing/invalid session and propagates genuine DB errors separately.
func (r *Repository) FindUserIDByHash(ctx context.Context, hash string) (string, error) {
	var userID string
	err := r.db.QueryRow(ctx, `
		SELECT user_id FROM sessions
		WHERE refresh_token_hash = $1
		  AND is_revoked = false
		  AND expires_at > now()
	`, hash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return userID, nil
}

// RotateSession atomically revokes an active session and creates its replacement,
// preventing the replay race where two concurrent refresh requests both succeed.
//
// The row is locked with SELECT … FOR UPDATE for the duration of the transaction.
// generate is called inside the transaction with the owning userID; it must
// return the new token hash, IP, user-agent and expiry to store.
// If generate returns an error, or any DB step fails, the transaction is rolled
// back and the old session remains valid.
func (r *Repository) RotateSession(
	ctx context.Context,
	oldHash string,
	generate func(userID string) (newHash, ip, ua string, expiresAt time.Time, err error),
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var userID string
	err = tx.QueryRow(ctx, `
		SELECT user_id FROM sessions
		WHERE refresh_token_hash = $1
		  AND is_revoked = false
		  AND expires_at > now()
		FOR UPDATE
	`, oldHash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	newHash, ip, ua, expiresAt, err := generate(userID)
	if err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE sessions SET is_revoked = true, last_used_at = now()
		WHERE refresh_token_hash = $1
	`, oldHash); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, ip_address, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, newHash, parseAddr(ip), ua, expiresAt); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Revoke marks a session as revoked and records the last-used timestamp.
func (r *Repository) Revoke(ctx context.Context, hash string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE sessions
		SET is_revoked = true, last_used_at = now()
		WHERE refresh_token_hash = $1
	`, hash)
	return err
}

// RevokeAllForUser revokes every active session belonging to the given user.
func (r *Repository) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE sessions SET is_revoked = true
		WHERE user_id = $1 AND is_revoked = false
	`, userID)
	return err
}

// SessionInfo is the read model for session listing (no token hash exposed).
type SessionInfo struct {
	ID         string     `json:"id"`
	IPAddress  string     `json:"ip_address"`
	UserAgent  string     `json:"user_agent"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	IsCurrent  bool       `json:"is_current"`
}

// ListForUser returns all active sessions for a user.
// currentHash is the SHA-256 hex of the caller's refresh token; matching rows
// have IsCurrent = true. Pass "" to skip current-session detection.
func (r *Repository) ListForUser(ctx context.Context, userID, currentHash string) ([]SessionInfo, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text,
		       COALESCE(ip_address::text, ''),
		       user_agent,
		       created_at,
		       last_used_at,
		       (refresh_token_hash = $2) AS is_current
		FROM sessions
		WHERE user_id = $1
		  AND is_revoked = false
		  AND expires_at > now()
		ORDER BY (refresh_token_hash = $2) DESC,
		         COALESCE(last_used_at, created_at) DESC
	`, userID, currentHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]SessionInfo, 0)
	for rows.Next() {
		var s SessionInfo
		var lastUsed *time.Time
		if err := rows.Scan(&s.ID, &s.IPAddress, &s.UserAgent, &s.CreatedAt, &lastUsed, &s.IsCurrent); err != nil {
			return nil, err
		}
		s.LastUsedAt = lastUsed
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// RevokeByID revokes a specific session owned by userID. Returns ErrNotFound if
// the session does not exist, is already revoked, or belongs to a different user.
func (r *Repository) RevokeByID(ctx context.Context, id, userID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE sessions SET is_revoked = true, last_used_at = now()
		WHERE id = $1 AND user_id = $2 AND is_revoked = false
	`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteExpired removes all expired and revoked sessions. Run periodically.
func (r *Repository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM sessions WHERE expires_at < now() OR is_revoked = true
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// parseAddr extracts the host from an addr string and converts it to netip.Addr.
// pgx v5 maps *netip.Addr to the PostgreSQL INET type; nil becomes NULL.
func parseAddr(addr string) *netip.Addr {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	return &a
}

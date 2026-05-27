package session

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("session_not_found")

const activeSessionWindowSQL = "2 minutes"

type Repository struct {
	db  *pgxpool.Pool
	hub *EventHub
}

func NewRepository(db *pgxpool.Pool, hub *EventHub) *Repository {
	return &Repository{db: db, hub: hub}
}

// Create inserts a new session. ip is the client address (host:port or bare host).
func (r *Repository) Create(ctx context.Context, userID, tokenHash, ip, userAgent string, deviceInfo map[string]string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, ip_address, user_agent, device_info, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID, tokenHash, parseAddr(ip), userAgent, normalizeDeviceInfo(deviceInfo), expiresAt)
	if err == nil {
		r.hub.Broadcast(userID)
	}
	return err
}

// RevokeForDevice closes older active sessions from the same device fingerprint
// before a new login creates the replacement session.
func (r *Repository) RevokeForDevice(ctx context.Context, userID, ip, userAgent string, deviceInfo map[string]string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE sessions
		SET is_revoked = true, last_used_at = now()
		WHERE user_id = $1
		  AND is_revoked = false
		  AND expires_at > now()
		  AND ip_address IS NOT DISTINCT FROM $2
		  AND user_agent = $3
		  AND COALESCE(device_info, '{}'::jsonb) = $4::jsonb
	`, userID, parseAddr(ip), userAgent, normalizeDeviceInfo(deviceInfo))
	if err == nil && tag.RowsAffected() > 0 {
		r.hub.Broadcast(userID)
	}
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
		  AND COALESCE(last_used_at, created_at) > now() - $2::interval
	`, hash, activeSessionWindowSQL).Scan(&userID)
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
// return the new token hash, IP, user-agent, device info and expiry to store.
// If generate returns an error, or any DB step fails, the transaction is rolled
// back and the old session remains valid.
func (r *Repository) RotateSession(
	ctx context.Context,
	oldHash string,
	generate func(userID string) (newHash, ip, ua string, deviceInfo map[string]string, expiresAt time.Time, err error),
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
		  AND COALESCE(last_used_at, created_at) > now() - $2::interval
		FOR UPDATE
	`, oldHash, activeSessionWindowSQL).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	newHash, ip, ua, deviceInfo, expiresAt, err := generate(userID)
	if err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE sessions SET is_revoked = true, last_used_at = now()
		WHERE refresh_token_hash = $1
	`, oldHash); err != nil {
		return err
	}

	// last_used_at = now() marks this as a rotated (confirmed-active) session so
	// the stale-session cleanup can distinguish it from a never-refreshed initial login.
	if _, err = tx.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, ip_address, user_agent, device_info, expires_at, last_used_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
	`, userID, newHash, parseAddr(ip), ua, normalizeDeviceInfo(deviceInfo), expiresAt); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Revoke marks a session as revoked and records the last-used timestamp.
func (r *Repository) Revoke(ctx context.Context, hash string) error {
	var userID string
	err := r.db.QueryRow(ctx, `
		UPDATE sessions
		SET is_revoked = true, last_used_at = now()
		WHERE refresh_token_hash = $1
		  AND is_revoked = false
		RETURNING user_id
	`, hash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	r.hub.Broadcast(userID)
	return err
}

// CheckReuse looks up whether the given token hash belongs to a session that
// was previously rotated (is_revoked = true). If so, it returns the owning
// userID — the caller should treat this as evidence of token theft and revoke
// all remaining sessions for that user.
func (r *Repository) CheckReuse(ctx context.Context, hash string) (string, error) {
	var userID string
	err := r.db.QueryRow(ctx, `
		SELECT user_id FROM sessions
		WHERE refresh_token_hash = $1 AND is_revoked = true
		LIMIT 1
	`, hash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return userID, nil
}

// RevokeAllForUser revokes every active session belonging to the given user.
func (r *Repository) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE sessions SET is_revoked = true
		WHERE user_id = $1 AND is_revoked = false
	`, userID)
	if err == nil {
		r.hub.BroadcastRevokedAll(userID)
	}
	return err
}

// RevokeOtherSessions revokes every active session for userID EXCEPT the one
// identified by currentHash. Used for "sign out all other devices".
func (r *Repository) RevokeOtherSessions(ctx context.Context, userID, currentHash string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE sessions SET is_revoked = true, last_used_at = now()
		WHERE user_id = $1
		  AND is_revoked = false
		  AND expires_at > now()
		  AND refresh_token_hash != $2
	`, userID, currentHash)
	if err == nil && tag.RowsAffected() > 0 {
		r.hub.BroadcastRevokedOthers(userID, currentHash)
	}
	return err
}

// EvictExcessSessions keeps only the `keep` most recently active sessions for
// the user and revokes the rest. Call with keep = maxAllowed-1 before creating
// a new session so the total never exceeds maxAllowed.
func (r *Repository) EvictExcessSessions(ctx context.Context, userID string, keep int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE sessions SET is_revoked = true
		WHERE user_id = $1
		  AND is_revoked = false
		  AND id NOT IN (
		      SELECT id FROM sessions
		      WHERE user_id = $1 AND is_revoked = false
		      ORDER BY COALESCE(last_used_at, created_at) DESC
		      LIMIT $2
		  )
	`, userID, keep)
	if err == nil {
		r.hub.Broadcast(userID)
	}
	return err
}

// SessionInfo is the read model for session listing (no token hash exposed).
type SessionInfo struct {
	ID         string            `json:"id"`
	IPAddress  string            `json:"ip_address"`
	UserAgent  string            `json:"user_agent"`
	DeviceInfo map[string]string `json:"device_info"`
	CreatedAt  time.Time         `json:"created_at"`
	LastUsedAt *time.Time        `json:"last_used_at"`
	IsCurrent  bool              `json:"is_current"`
}

// ListForUser returns all active sessions for a user.
// currentHash is the SHA-256 hex of the caller's refresh token; matching rows
// have IsCurrent = true. Pass "" to skip current-session detection.
func (r *Repository) ListForUser(ctx context.Context, userID, currentHash string) ([]SessionInfo, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text,
		       COALESCE(ip_address::text, ''),
		       user_agent,
		       COALESCE(device_info, '{}'::jsonb),
		       created_at,
		       last_used_at,
		       (refresh_token_hash = $2) AS is_current
		FROM sessions
		WHERE user_id = $1
		  AND is_revoked = false
		  AND expires_at > now()
		  AND COALESCE(last_used_at, created_at) > now() - $3::interval
		ORDER BY (refresh_token_hash = $2) DESC,
		         COALESCE(last_used_at, created_at) DESC
	`, userID, currentHash, activeSessionWindowSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]SessionInfo, 0)
	for rows.Next() {
		var s SessionInfo
		var lastUsed *time.Time
		var deviceInfo []byte
		if err := rows.Scan(&s.ID, &s.IPAddress, &s.UserAgent, &deviceInfo, &s.CreatedAt, &lastUsed, &s.IsCurrent); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(deviceInfo, &s.DeviceInfo); err != nil {
			s.DeviceInfo = map[string]string{}
		}
		s.LastUsedAt = lastUsed
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func normalizeDeviceInfo(deviceInfo map[string]string) []byte {
	if len(deviceInfo) == 0 {
		return []byte("{}")
	}

	clean := make(map[string]string, len(deviceInfo))
	for key, value := range deviceInfo {
		if key == "" || len(key) > 40 || len(value) > 80 {
			continue
		}
		clean[key] = value
	}
	if len(clean) == 0 {
		return []byte("{}")
	}

	b, err := json.Marshal(clean)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// RevokeByID revokes a specific session owned by userID. Returns ErrNotFound if
// the session does not exist, is already revoked, or belongs to a different user.
func (r *Repository) RevokeByID(ctx context.Context, id, userID string) error {
	var tokenHash string
	err := r.db.QueryRow(ctx, `
		UPDATE sessions SET is_revoked = true, last_used_at = now()
		WHERE id = $1 AND user_id = $2 AND is_revoked = false
		RETURNING refresh_token_hash
	`, id, userID).Scan(&tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	r.hub.BroadcastRevoked(userID, tokenHash)
	r.hub.Broadcast(userID)
	return nil
}

// staleInitialCutoffSQL is the time window after which a session whose access
// token was never refreshed is considered stale and will be force-revoked.
// AccessTTL = 1 min; frontend refreshes at 80% (~48 s); 90 s gives a generous
// buffer for slow clients while still catching abandoned/bot logins quickly.
const staleInitialCutoffSQL = "90 seconds"

// RevokeStaleInitialSessions finds sessions that were created more than
// staleInitialCutoffSQL ago and whose last_used_at is still NULL — meaning the
// access token was never refreshed. These are abandoned logins (e.g. the browser
// tab closed immediately, or a credential-stuffing bot that never followed up).
// For each revoked session BroadcastRevoked is called so any still-open SSE
// connection is notified; other sessions for the same user see a "change" event
// (via the SSE handler fall-through) and can update their session list.
func (r *Repository) RevokeStaleInitialSessions(ctx context.Context) (int64, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE sessions SET is_revoked = true, last_used_at = now()
		WHERE is_revoked = false
		  AND expires_at > now()
		  AND last_used_at IS NULL
		  AND created_at < now() - $1::interval
		  AND created_at > now() - interval '10 minutes'
		RETURNING user_id, refresh_token_hash
	`, staleInitialCutoffSQL)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		var userID, tokenHash string
		if err := rows.Scan(&userID, &tokenHash); err != nil {
			return count, err
		}
		count++
		// Notify the stale session itself (if SSE still open) and all other
		// sessions for this user (they'll see "change" → refresh list → snackbar).
		r.hub.BroadcastRevoked(userID, tokenHash)
	}
	return count, rows.Err()
}

// DeleteExpired removes all expired and revoked sessions. Run periodically.
func (r *Repository) DeleteExpired(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM sessions
		WHERE expires_at < now()
		   OR is_revoked = true
		   OR COALESCE(last_used_at, created_at) <= now() - $1::interval
	`, activeSessionWindowSQL)
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

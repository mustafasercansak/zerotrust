package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const OIDCRefreshTTL = 30 * 24 * time.Hour

var ErrRefreshTokenNotFound = errors.New("refresh_token_not_found")

// ErrRefreshTokenReused is returned when an already-consumed refresh token is
// presented again. Per RFC 6819 §5.2.2.3 the whole grant chain (token family)
// is revoked as a result.
var ErrRefreshTokenReused = errors.New("refresh_token_reused")

// OIDCRefreshSession holds the data associated with an OIDC refresh token.
type OIDCRefreshSession struct {
	UserID   string    `json:"user_id"`
	ClientID string    `json:"client_id"`
	Scopes   []string  `json:"scopes"`
	AuthTime time.Time `json:"auth_time"`
	// FamilyID identifies the grant chain this token belongs to. All tokens
	// derived from one authorization-code exchange share it; reuse of any
	// consumed token revokes every live token in the family.
	FamilyID string `json:"family_id,omitempty"`
}

// RefreshTokenStore manages opaque OIDC refresh tokens in Redis.
// Each token is single-use (consumed on read) and rotated on every refresh.
//
// Besides the token key itself (oauth2:refresh:<hash>) the store maintains:
//   - a per-user index (oauth2:refresh:user:<uid>) so RevokeAllForUser can
//     invalidate every OIDC refresh token a user holds (#82);
//   - a per-family index (oauth2:refresh:family:<fid>) plus a tombstone for
//     consumed tokens (oauth2:refresh:used:<hash>) for RFC 6819 §5.2.2.3
//     reuse detection (#85).
type RefreshTokenStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRefreshTokenStore(rdb *redis.Client) *RefreshTokenStore {
	return &RefreshTokenStore{rdb: rdb, ttl: OIDCRefreshTTL}
}

// Save generates a new opaque refresh token, stores its hash, and returns the
// plaintext token (to be sent to the client once and never stored again).
// A session without a FamilyID starts a new grant chain.
func (s *RefreshTokenStore) Save(ctx context.Context, session *OIDCRefreshSession) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	if session.FamilyID == "" {
		session.FamilyID = uuid.NewString()
	}

	data, err := json.Marshal(session)
	if err != nil {
		return "", err
	}

	hash := s.hash(token)
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, s.key(hash), data, s.ttl)
	pipe.SAdd(ctx, s.userKey(session.UserID), hash)
	pipe.Expire(ctx, s.userKey(session.UserID), s.ttl)
	pipe.SAdd(ctx, s.familyKey(session.FamilyID), hash)
	pipe.Expire(ctx, s.familyKey(session.FamilyID), s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}
	return token, nil
}

// GetAndConsume atomically retrieves and deletes the session for token.
// Returns ErrRefreshTokenNotFound if the token is unknown, and
// ErrRefreshTokenReused (after revoking the whole token family) if the token
// was already consumed before.
func (s *RefreshTokenStore) GetAndConsume(ctx context.Context, token string) (*OIDCRefreshSession, error) {
	hash := s.hash(token)
	val, err := s.rdb.GetDel(ctx, s.key(hash)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, s.handleMiss(ctx, hash)
		}
		return nil, err
	}
	var sess OIDCRefreshSession
	if err := json.Unmarshal([]byte(val), &sess); err != nil {
		return nil, err
	}
	// Tombstone the consumed token so a later presentation is detected as
	// reuse, and drop it from the live indexes.
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, s.usedKey(hash), val, s.ttl)
	pipe.SRem(ctx, s.userKey(sess.UserID), hash)
	pipe.SRem(ctx, s.familyKey(sess.FamilyID), hash)
	pipe.Exec(ctx) //nolint:errcheck
	return &sess, nil
}

// handleMiss runs on an unknown token hash: if the hash has a tombstone the
// token was already consumed — evidence of theft — so the entire grant chain
// is revoked and ErrRefreshTokenReused is returned.
func (s *RefreshTokenStore) handleMiss(ctx context.Context, hash string) error {
	val, err := s.rdb.Get(ctx, s.usedKey(hash)).Result()
	if err != nil {
		return ErrRefreshTokenNotFound
	}
	var sess OIDCRefreshSession
	if err := json.Unmarshal([]byte(val), &sess); err != nil {
		return ErrRefreshTokenNotFound
	}
	s.revokeFamily(ctx, sess.FamilyID, sess.UserID)
	return ErrRefreshTokenReused
}

// revokeFamily deletes every live token of a grant chain and cleans up the
// per-user and per-family indexes.
func (s *RefreshTokenStore) revokeFamily(ctx context.Context, familyID, userID string) {
	if familyID == "" {
		return
	}
	fkey := s.familyKey(familyID)
	hashes, err := s.rdb.SMembers(ctx, fkey).Result()
	if err == nil && len(hashes) > 0 {
		keys := make([]string, 0, len(hashes))
		for _, h := range hashes {
			keys = append(keys, s.key(h))
		}
		s.rdb.Del(ctx, keys...)                    //nolint:errcheck
		s.rdb.SRem(ctx, s.userKey(userID), hashes) //nolint:errcheck
	}
	s.rdb.Del(ctx, fkey) //nolint:errcheck
}

// Peek retrieves the session for token without consuming it.
func (s *RefreshTokenStore) Peek(ctx context.Context, token string) (*OIDCRefreshSession, error) {
	val, err := s.rdb.Get(ctx, s.key(s.hash(token))).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrRefreshTokenNotFound
		}
		return nil, err
	}
	var sess OIDCRefreshSession
	if err := json.Unmarshal([]byte(val), &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// Delete removes a token and its index entries without leaving a reuse
// tombstone. Used for explicit revocation (RFC 7009), which must not later
// be mistaken for token-theft reuse.
func (s *RefreshTokenStore) Delete(ctx context.Context, token string) error {
	hash := s.hash(token)
	val, err := s.rdb.GetDel(ctx, s.key(hash)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return err
	}
	var sess OIDCRefreshSession
	if err := json.Unmarshal([]byte(val), &sess); err != nil {
		return nil
	}
	pipe := s.rdb.Pipeline()
	pipe.SRem(ctx, s.userKey(sess.UserID), hash)
	pipe.SRem(ctx, s.familyKey(sess.FamilyID), hash)
	_, err = pipe.Exec(ctx)
	return err
}

// RevokeAllForUser deletes every OIDC refresh token belonging to userID.
// Called whenever all of a user's sessions are revoked (password change,
// admin revoke-all, token-reuse response) so OIDC grants die with them (#82).
func (s *RefreshTokenStore) RevokeAllForUser(ctx context.Context, userID string) error {
	ukey := s.userKey(userID)
	hashes, err := s.rdb.SMembers(ctx, ukey).Result()
	if err != nil {
		return err
	}
	if len(hashes) > 0 {
		keys := make([]string, 0, len(hashes)+1)
		for _, h := range hashes {
			keys = append(keys, s.key(h))
		}
		keys = append(keys, ukey)
		return s.rdb.Del(ctx, keys...).Err()
	}
	return s.rdb.Del(ctx, ukey).Err()
}

func (s *RefreshTokenStore) hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *RefreshTokenStore) key(hash string) string {
	return fmt.Sprintf("oauth2:refresh:%s", hash)
}

func (s *RefreshTokenStore) usedKey(hash string) string {
	return fmt.Sprintf("oauth2:refresh:used:%s", hash)
}

func (s *RefreshTokenStore) userKey(userID string) string {
	return fmt.Sprintf("oauth2:refresh:user:%s", userID)
}

func (s *RefreshTokenStore) familyKey(familyID string) string {
	return fmt.Sprintf("oauth2:refresh:family:%s", familyID)
}

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

	"github.com/redis/go-redis/v9"
)

const OIDCRefreshTTL = 30 * 24 * time.Hour

var ErrRefreshTokenNotFound = errors.New("refresh_token_not_found")

// OIDCRefreshSession holds the data associated with an OIDC refresh token.
type OIDCRefreshSession struct {
	UserID   string    `json:"user_id"`
	ClientID string    `json:"client_id"`
	Scopes   []string  `json:"scopes"`
	AuthTime time.Time `json:"auth_time"`
}

// RefreshTokenStore manages opaque OIDC refresh tokens in Redis.
// Each token is single-use (consumed on read) and rotated on every refresh.
type RefreshTokenStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRefreshTokenStore(rdb *redis.Client) *RefreshTokenStore {
	return &RefreshTokenStore{rdb: rdb, ttl: OIDCRefreshTTL}
}

// Save generates a new opaque refresh token, stores its hash, and returns the
// plaintext token (to be sent to the client once and never stored again).
func (s *RefreshTokenStore) Save(ctx context.Context, session *OIDCRefreshSession) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)

	data, err := json.Marshal(session)
	if err != nil {
		return "", err
	}

	key := s.key(token)
	if err := s.rdb.Set(ctx, key, data, s.ttl).Err(); err != nil {
		return "", err
	}
	return token, nil
}

// GetAndConsume atomically retrieves and deletes the session for token.
// Returns ErrRefreshTokenNotFound if the token is unknown or already used.
func (s *RefreshTokenStore) GetAndConsume(ctx context.Context, token string) (*OIDCRefreshSession, error) {
	val, err := s.rdb.GetDel(ctx, s.key(token)).Result()
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

func (s *RefreshTokenStore) key(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("oauth2:refresh:%s", hex.EncodeToString(sum[:]))
}

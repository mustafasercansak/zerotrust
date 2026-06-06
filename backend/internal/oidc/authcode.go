package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrAuthCodeNotFound = errors.New("auth_code_not_found")
var ErrAuthCodeExpired = errors.New("auth_code_expired")

type AuthCodeSession struct {
	Code                string    `json:"code"`
	UserID              string    `json:"user_id"`
	ClientID            string    `json:"client_id"`
	RedirectURI         string    `json:"redirect_uri"`
	Scopes              []string  `json:"scopes"`
	CodeChallenge       string    `json:"code_challenge,omitempty"`
	CodeChallengeMethod string    `json:"code_challenge_method,omitempty"`
	Nonce               string    `json:"nonce,omitempty"`
	AuthTime            time.Time `json:"auth_time"`
}

type AuthCodeStore struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewAuthCodeStore(rdb *redis.Client) *AuthCodeStore {
	return &AuthCodeStore{
		rdb: rdb,
		ttl: 5 * time.Minute,
	}
}

func (s *AuthCodeStore) Save(ctx context.Context, session *AuthCodeSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("oauth2:code:%s", session.Code)
	return s.rdb.Set(ctx, key, data, s.ttl).Err()
}

func (s *AuthCodeStore) GetAndConsume(ctx context.Context, code string) (*AuthCodeSession, error) {
	key := fmt.Sprintf("oauth2:code:%s", code)
	val, err := s.rdb.GetDel(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrAuthCodeNotFound
		}
		return nil, err
	}

	var session AuthCodeSession
	if err := json.Unmarshal([]byte(val), &session); err != nil {
		return nil, err
	}

	return &session, nil
}

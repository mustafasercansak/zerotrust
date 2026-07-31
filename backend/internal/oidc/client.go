package oidc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var ErrClientNotFound = errors.New("client_not_found")
var ErrInvalidClientSecret = errors.New("invalid_client_secret")

type Client struct {
	ID               string    `json:"id"`
	ClientID         string    `json:"client_id"`
	ClientSecretHash string    `json:"-"`
	Name             string    `json:"name"`
	RedirectURIs     []string  `json:"redirect_uris"`
	AllowedScopes    []string  `json:"allowed_scopes"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ClientRepository struct {
	db *pgxpool.Pool
}

func NewClientRepository(db *pgxpool.Pool) *ClientRepository {
	return &ClientRepository{db: db}
}

// FindByClientID searches for a client by its client_id
func (r *ClientRepository) FindByClientID(ctx context.Context, clientID string) (*Client, error) {
	var c Client
	err := r.db.QueryRow(ctx, `
		SELECT id, client_id, client_secret_hash, name, redirect_uris, allowed_scopes, created_at, updated_at
		FROM oauth2_clients
		WHERE client_id = $1
	`, clientID).Scan(&c.ID, &c.ClientID, &c.ClientSecretHash, &c.Name, &c.RedirectURIs, &c.AllowedScopes, &c.CreatedAt, &c.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrClientNotFound
		}
		return nil, err
	}
	return &c, nil
}

// AuthenticateClient checks if a client ID and secret match.
func (r *ClientRepository) AuthenticateClient(ctx context.Context, clientID, clientSecret string) (*Client, error) {
	c, err := r.FindByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(c.ClientSecretHash), []byte(clientSecret))
	if err != nil {
		return nil, ErrInvalidClientSecret
	}

	return c, nil
}

// ValidateRedirectURI checks if the requested redirect URI matches any registered ones
func (c *Client) ValidateRedirectURI(uri string) bool {
	for _, u := range c.RedirectURIs {
		if u == uri {
			return true
		}
	}
	return false
}

// ValidateScope checks if requested scopes are allowed for this client
func (c *Client) ValidateScope(scopes []string) bool {
	allowed := make(map[string]bool)
	for _, s := range c.AllowedScopes {
		allowed[s] = true
	}
	for _, s := range scopes {
		if !allowed[s] {
			return false
		}
	}
	return true
}

// List returns all registered OAuth2 clients
func (r *ClientRepository) List(ctx context.Context) ([]*Client, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, client_id, client_secret_hash, name, redirect_uris, allowed_scopes, created_at, updated_at
		FROM oauth2_clients
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Client
	for rows.Next() {
		var c Client
		err := rows.Scan(&c.ID, &c.ClientID, &c.ClientSecretHash, &c.Name, &c.RedirectURIs, &c.AllowedScopes, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, err
		}
		list = append(list, &c)
	}
	return list, rows.Err()
}

// Create inserts a new client into the database
func (r *ClientRepository) Create(ctx context.Context, clientID, secretHash, name string, redirectURIs, allowedScopes []string) (*Client, error) {
	var c Client
	err := r.db.QueryRow(ctx, `
		INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, client_id, client_secret_hash, name, redirect_uris, allowed_scopes, created_at, updated_at
	`, clientID, secretHash, name, redirectURIs, allowedScopes).Scan(&c.ID, &c.ClientID, &c.ClientSecretHash, &c.Name, &c.RedirectURIs, &c.AllowedScopes, &c.CreatedAt, &c.UpdatedAt)

	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Delete removes a client by its UUID and returns its client_id (for auditing).
func (r *ClientRepository) Delete(ctx context.Context, id string) (string, error) {
	var clientID string
	err := r.db.QueryRow(ctx, `DELETE FROM oauth2_clients WHERE id = $1 RETURNING client_id`, id).Scan(&clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrClientNotFound
		}
		return "", err
	}
	return clientID, nil
}

// RotateSecret generates a new random client secret, stores its bcrypt hash, and
// returns the plaintext secret (shown once).
func (r *ClientRepository) RotateSecret(ctx context.Context, id string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(raw)
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), 12)
	if err != nil {
		return "", err
	}
	tag, err := r.db.Exec(ctx, `UPDATE oauth2_clients SET client_secret_hash = $1, updated_at = NOW() WHERE id = $2`, string(hash), id)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", ErrClientNotFound
	}
	return secret, nil
}

// Update modifies mutable fields of an OIDC client by UUID
func (r *ClientRepository) Update(ctx context.Context, id, name string, redirectURIs, allowedScopes []string) (*Client, error) {
	var c Client
	err := r.db.QueryRow(ctx, `
		UPDATE oauth2_clients
		SET name = $2, redirect_uris = $3, allowed_scopes = $4, updated_at = NOW()
		WHERE id = $1
		RETURNING id, client_id, client_secret_hash, name, redirect_uris, allowed_scopes, created_at, updated_at
	`, id, name, redirectURIs, allowedScopes).Scan(&c.ID, &c.ClientID, &c.ClientSecretHash, &c.Name, &c.RedirectURIs, &c.AllowedScopes, &c.CreatedAt, &c.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrClientNotFound
		}
		return nil, err
	}
	return &c, nil
}


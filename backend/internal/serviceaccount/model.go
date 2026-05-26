package serviceaccount

import "time"

type ServiceAccount struct {
	ID               string     `db:"id"`
	Name             string     `db:"name"`
	ClientID         string     `db:"client_id"`
	ClientSecretHash string     `db:"client_secret_hash"`
	IsActive         bool       `db:"is_active"`
	CreatedAt        time.Time  `db:"created_at"`
	ExpiresAt        *time.Time `db:"expires_at"`
	Scopes           []string
}

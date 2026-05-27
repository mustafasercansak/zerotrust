package user

import (
	"errors"
	"time"
)

var ErrInvalidProfile = errors.New("invalid_profile")

type User struct {
	ID           string    `db:"id"`
	Email        string    `db:"email"`
	FirstName    string    `db:"first_name"`
	LastName     string    `db:"last_name"`
	HasAvatar    bool      `db:"has_avatar"`
	PasswordHash string    `db:"password_hash"`
	Locale       string    `db:"locale"`
	IsActive     bool      `db:"is_active"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
	Roles        []string
}

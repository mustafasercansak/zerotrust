package user

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("user_not_found")
var ErrEmailTaken = errors.New("email_taken")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, email, passwordHash, locale string) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, locale)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, locale, is_active, created_at, updated_at
	`, email, passwordHash, locale).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	return u, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, locale, is_active, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Roles, _ = r.getRoles(ctx, u.ID)
	return u, nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, locale, is_active, created_at, updated_at
		FROM users WHERE email = $1
	`, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.Roles, _ = r.getRoles(ctx, u.ID)
	return u, nil
}

// ListAll returns all users with their roles, ordered by creation time.
func (r *Repository) ListAll(ctx context.Context) ([]*User, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.email, u.locale, u.is_active, u.created_at, u.updated_at,
		       COALESCE(string_agg(ro.name, ',' ORDER BY ro.name), '') AS roles
		FROM users u
		LEFT JOIN user_roles ur ON u.id = ur.user_id
		LEFT JOIN roles ro ON ur.role_id = ro.id
		GROUP BY u.id
		ORDER BY u.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		var rolesStr string
		if err := rows.Scan(&u.ID, &u.Email, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt, &rolesStr); err != nil {
			return nil, err
		}
		if rolesStr != "" {
			u.Roles = strings.Split(rolesStr, ",")
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// AssignRoleByName assigns a named role to a user (idempotent).
func (r *Repository) AssignRoleByName(ctx context.Context, userID, roleName string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE name = $2
		ON CONFLICT DO NOTHING
	`, userID, roleName)
	return err
}

// SetRoles replaces all roles for a user.
func (r *Repository) SetRoles(ctx context.Context, userID string, roleNames []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, name := range roleNames {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id)
			SELECT $1, id FROM roles WHERE name = $2
			ON CONFLICT DO NOTHING
		`, userID, name); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// GetPermissions returns all resource:action strings for the user via their roles.
func (r *Repository) GetPermissions(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT CONCAT(p.resource, ':', p.action)
		FROM permissions p
		JOIN role_permissions rp ON p.id = rp.permission_id
		JOIN user_roles ur ON rp.role_id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY 1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var perms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

func (r *Repository) getRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ro.name FROM roles ro
		JOIN user_roles ur ON ro.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY ro.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		roles = append(roles, name)
	}
	return roles, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

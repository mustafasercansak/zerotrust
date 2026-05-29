package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("user_not_found")
var ErrEmailTaken = errors.New("email_taken")
var ErrUnknownRole = errors.New("unknown_role")

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
		RETURNING id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, is_active, created_at, updated_at
	`, email, passwordHash, locale).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	return u, nil
}

// CreateWithRoles creates a user and assigns roles atomically.
func (r *Repository) CreateWithRoles(ctx context.Context, email, passwordHash, locale string, roleNames []string) (*User, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	u := &User{}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, locale)
		VALUES ($1, $2, $3)
		RETURNING id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, is_active, created_at, updated_at
	`, email, passwordHash, locale).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}

	for _, name := range roleNames {
		tag, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id)
			SELECT $1, id FROM roles WHERE name = $2
		`, u.ID, name)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			return nil, fmt.Errorf("%w: %q", ErrUnknownRole, name)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	u.Roles = roleNames
	return u, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(ctx, `
		SELECT id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, is_active, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	roles, err := r.getRoles(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.Roles = roles
	return u, nil
}

func (r *Repository) UpdateLocale(ctx context.Context, userID, locale string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET locale = $1, updated_at = NOW() WHERE id = $2`,
		locale, userID,
	)
	return err
}

func (r *Repository) UpdateProfile(ctx context.Context, userID, firstName, lastName string) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(ctx, `
		UPDATE users
		SET first_name = $1, last_name = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, is_active, created_at, updated_at
	`, firstName, lastName, userID).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	roles, err := r.getRoles(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.Roles = roles
	return u, nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(ctx, `
		SELECT id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, is_active, created_at, updated_at
		FROM users WHERE email = $1
	`, email).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	roles, err := r.getRoles(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.Roles = roles
	return u, nil
}

// ListParams configures pagination, sorting, and filtering for List.
type ListParams struct {
	Limit   int
	Offset  int
	SortBy  string // email | created_at | is_active
	SortDir string // asc | desc
	Email   string // ILIKE filter
	Status  string // active | inactive | ""
}

// ListResult holds one page of users and the unfiltered total count.
type ListResult struct {
	Users          []*User
	ActiveSessions map[string]int // userID → active session count
	Total          int
}

var userSortCols = map[string]string{
	"email":      "u.email",
	"created_at": "u.created_at",
	"is_active":  "u.is_active",
}

// List returns a filtered, sorted, paginated page of users with the total matching count.
func (r *Repository) List(ctx context.Context, p ListParams) (ListResult, error) {
	if p.Limit <= 0 || p.Limit > 200 {
		p.Limit = 25
	}
	col, ok := userSortCols[p.SortBy]
	if !ok {
		col = "u.created_at"
	}
	dir := "ASC"
	if strings.EqualFold(p.SortDir, "desc") {
		dir = "DESC"
	}

	var conds []string
	var args []any
	n := 1

	if p.Email != "" {
		conds = append(conds, fmt.Sprintf("LOWER(u.email) LIKE $%d", n))
		args = append(args, "%"+strings.ToLower(p.Email)+"%")
		n++
	}
	switch p.Status {
	case "active":
		conds = append(conds, fmt.Sprintf("u.is_active = $%d", n))
		args = append(args, true)
		n++
	case "inactive":
		conds = append(conds, fmt.Sprintf("u.is_active = $%d", n))
		args = append(args, false)
		n++
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM users u %s`, where),
		args...,
	).Scan(&total); err != nil {
		return ListResult{}, err
	}

	dataArgs := append(append([]any{}, args...), p.Limit, p.Offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT u.id, u.email, u.first_name, u.last_name, u.avatar_object_key <> '', u.locale, u.is_active, u.created_at, u.updated_at,
		       COALESCE(string_agg(ro.name, ',' ORDER BY ro.name), '') AS roles,
		       (SELECT COUNT(*)
		        FROM sessions s
		        WHERE s.user_id = u.id
		          AND s.is_revoked = false
		          AND s.expires_at > now()
		          AND COALESCE(s.last_used_at, s.created_at) > now() - interval '2 minutes') AS active_sessions
		FROM users u
		LEFT JOIN user_roles ur ON u.id = ur.user_id
		LEFT JOIN roles ro ON ur.role_id = ro.id
		%s
		GROUP BY u.id
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, where, col, dir, n, n+1), dataArgs...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	users := make([]*User, 0)
	activeSessions := make(map[string]int)
	for rows.Next() {
		u := &User{}
		var rolesStr string
		var sessionCount int
		if err := rows.Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt, &rolesStr, &sessionCount); err != nil {
			return ListResult{}, err
		}
		if rolesStr != "" {
			u.Roles = strings.Split(rolesStr, ",")
		} else {
			u.Roles = []string{}
		}
		users = append(users, u)
		activeSessions[u.ID] = sessionCount
	}
	return ListResult{Users: users, ActiveSessions: activeSessions, Total: total}, rows.Err()
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
		tag, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id)
			SELECT $1, id FROM roles WHERE name = $2
		`, userID, name)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w: %q", ErrUnknownRole, name)
		}
	}
	return tx.Commit(ctx)
}

// SetActive activates or deactivates a user account.
func (r *Repository) SetActive(ctx context.Context, userID string, active bool) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2`,
		active, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdatePassword replaces the stored password hash for a user.
func (r *Repository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2
	`, passwordHash, userID)
	return err
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

func (r *Repository) UpdateAvatar(ctx context.Context, userID, key string, size int) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(ctx, `
		UPDATE users
		SET avatar_object_key = $1, avatar_size = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, is_active, created_at, updated_at
	`, key, size, userID).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	roles, err := r.getRoles(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.Roles = roles
	return u, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}


package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// ErrLastAdmin is returned when an operation would leave the system with no
// active admin (demoting or deactivating the last one). See ISSUE_LIST #34.
var ErrLastAdmin = errors.New("last_admin")

// adminRoleName is the role that must always have at least one active holder.
const adminRoleName = "admin"

type Repository struct {
	db        *pgxpool.Pool
	secClient encrypter
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type encrypter interface {
	EncryptData(ctx context.Context, plaintext string) (string, error)
	DecryptData(ctx context.Context, ciphertext string) (string, error)
}

func (r *Repository) SetSecretsClient(c encrypter) {
	r.secClient = c
}

func (r *Repository) encrypt(ctx context.Context, val string) (string, error) {
	if r.secClient == nil || val == "" {
		return val, nil
	}
	return r.secClient.EncryptData(ctx, val)
}

func (r *Repository) decrypt(ctx context.Context, val string) (string, error) {
	if r.secClient == nil || val == "" {
		return val, nil
	}
	return r.secClient.DecryptData(ctx, val)
}

func (r *Repository) decryptUser(ctx context.Context, u *User) error {
	if u == nil {
		return nil
	}
	var err error
	u.Email, err = r.decrypt(ctx, u.Email)
	if err != nil {
		return err
	}
	u.FirstName, err = r.decrypt(ctx, u.FirstName)
	if err != nil {
		return err
	}
	u.LastName, err = r.decrypt(ctx, u.LastName)
	if err != nil {
		return err
	}
	return nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(s))))
	return hex.EncodeToString(h[:])
}

func (r *Repository) Create(ctx context.Context, email, passwordHash, locale string) (*User, error) {
	encEmail, err := r.encrypt(ctx, email)
	if err != nil {
		return nil, err
	}
	emailHash := sha256Hex(email)

	u := &User{}
	err = r.db.QueryRow(ctx, `
		INSERT INTO users (email, email_hash, password_hash, locale)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, notify_security_emails, is_active, created_at, updated_at
	`, encEmail, emailHash, passwordHash, locale).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.NotifySecurityEmails, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	if err := r.decryptUser(ctx, u); err != nil {
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

	encEmail, err := r.encrypt(ctx, email)
	if err != nil {
		return nil, err
	}
	emailHash := sha256Hex(email)

	u := &User{}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, email_hash, password_hash, locale)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, notify_security_emails, is_active, created_at, updated_at
	`, encEmail, emailHash, passwordHash, locale).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.NotifySecurityEmails, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
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

	if err := r.decryptUser(ctx, u); err != nil {
		return nil, err
	}

	u.Roles = roleNames
	return u, nil
}

func (r *Repository) FindByID(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(ctx, `
		SELECT id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, notify_security_emails, is_active, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.NotifySecurityEmails, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
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
	if err := r.decryptUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Repository) UpdateLocale(ctx context.Context, userID, locale string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET locale = $1, updated_at = NOW() WHERE id = $2`,
		locale, userID,
	)
	return err
}

func (r *Repository) UpdateNotifySecurityEmails(ctx context.Context, userID string, enabled bool) error {
	_, err := r.db.Exec(ctx,
		`UPDATE users SET notify_security_emails = $1, updated_at = NOW() WHERE id = $2`,
		enabled, userID,
	)
	return err
}

func (r *Repository) UpdateProfile(ctx context.Context, userID, firstName, lastName string) (*User, error) {
	encFirstName, err := r.encrypt(ctx, firstName)
	if err != nil {
		return nil, err
	}
	encLastName, err := r.encrypt(ctx, lastName)
	if err != nil {
		return nil, err
	}

	u := &User{}
	err = r.db.QueryRow(ctx, `
		UPDATE users
		SET first_name = $1, last_name = $2, updated_at = NOW()
		WHERE id = $3
		RETURNING id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, notify_security_emails, is_active, created_at, updated_at
	`, encFirstName, encLastName, userID).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.NotifySecurityEmails, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
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
	if err := r.decryptUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	emailHash := sha256Hex(email)
	u := &User{}
	err := r.db.QueryRow(ctx, `
		SELECT id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, notify_security_emails, is_active, created_at, updated_at
		FROM users WHERE email_hash = $1
	`, emailHash).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.NotifySecurityEmails, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
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
	if err := r.decryptUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// ListParams configures pagination, sorting, and filtering for List.
type ListParams struct {
	Limit   int
	Offset  int
	SortBy  string // email | created_at | is_active
	SortDir string // asc | desc
	Email   string // exact match via email_hash
	Status  string // active | inactive | ""
	Role    string // exact role name filter
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
		conds = append(conds, fmt.Sprintf("u.email_hash = $%d", n))
		args = append(args, sha256Hex(p.Email))
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
	if p.Role != "" {
		conds = append(conds, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = u.id AND r.name = $%d)`, n,
		))
		args = append(args, p.Role)
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
		if err := r.decryptUser(ctx, u); err != nil {
			return ListResult{}, err
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

	// Validate every requested role exists first, so an unknown role is reported
	// (ErrUnknownRole) regardless of the last-admin guard below — input
	// validation takes precedence over the invariant check.
	newHasAdmin := false
	for _, name := range roleNames {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM roles WHERE name = $1)`, name).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: %q", ErrUnknownRole, name)
		}
		if name == adminRoleName {
			newHasAdmin = true
		}
	}

	// Enforce the last-admin invariant before mutating: if the new role set
	// drops admin and the target is currently the only active admin, reject.
	// The check must run against pre-change state. (ISSUE_LIST #34)
	if !newHasAdmin {
		if err := guardLastAdmin(ctx, tx, userID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, name := range roleNames {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id)
			SELECT $1, id FROM roles WHERE name = $2
		`, userID, name); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// SetActive activates or deactivates a user account.
func (r *Repository) SetActive(ctx context.Context, userID string, active bool) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Deactivating the only active admin would lock the system out. Check
	// before mutating; this rolls back trivially if it fails. (ISSUE_LIST #34)
	if !active {
		if err := guardLastAdmin(ctx, tx, userID); err != nil {
			return err
		}
	}

	tag, err := tx.Exec(ctx,
		`UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2`,
		active, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return tx.Commit(ctx)
}

// guardLastAdmin returns ErrLastAdmin when the target user is currently the
// sole active admin — i.e. removing their admin role or deactivating them would
// leave the system with no active admin. It is a no-op when the target is not
// an active admin, or when another active admin exists.
func guardLastAdmin(ctx context.Context, tx pgx.Tx, targetUserID string) error {
	var targetIsActiveAdmin bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM users u
			JOIN user_roles ur ON u.id = ur.user_id
			JOIN roles ro ON ur.role_id = ro.id
			WHERE u.id = $1::uuid AND u.is_active = true AND ro.name = $2
		)
	`, targetUserID, adminRoleName).Scan(&targetIsActiveAdmin); err != nil {
		return err
	}
	if !targetIsActiveAdmin {
		return nil
	}

	var otherActiveAdmins int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(DISTINCT u.id)
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
		JOIN roles ro ON ur.role_id = ro.id
		WHERE ro.name = $1 AND u.is_active = true AND u.id <> $2::uuid
	`, adminRoleName, targetUserID).Scan(&otherActiveAdmins); err != nil {
		return err
	}
	if otherActiveAdmins == 0 {
		return ErrLastAdmin
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
		RETURNING id, email, first_name, last_name, avatar_object_key <> '', password_hash, locale, notify_security_emails, is_active, created_at, updated_at
	`, key, size, userID).Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.HasAvatar, &u.PasswordHash, &u.Locale, &u.NotifySecurityEmails, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
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
	if err := r.decryptUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// SecurityPostureStats holds platform-wide security posture counts for the admin home page.
type SecurityPostureStats struct {
	TotalUsers       int `json:"total_users"`
	UsersWithoutMFA  int `json:"users_without_mfa"`
	UsersInactive30d int `json:"users_inactive_30d"`
}

// SecurityPosture returns a snapshot of platform security health in a single query.
func (r *Repository) SecurityPosture(ctx context.Context) (SecurityPostureStats, error) {
	var s SecurityPostureStats
	err := r.db.QueryRow(ctx, `
		SELECT
		  COUNT(*)                                                    AS total,
		  COUNT(*) FILTER (
		    WHERE NOT EXISTS (
		      SELECT 1 FROM user_mfa m
		      WHERE m.user_id = u.id AND m.enabled_at IS NOT NULL
		    )
		    AND NOT EXISTS (
		      SELECT 1 FROM user_webauthn_credentials w
		      WHERE w.user_id = u.id
		    )
		  )                                                           AS without_mfa,
		  COUNT(*) FILTER (
		    WHERE NOT EXISTS (
		      SELECT 1 FROM audit_logs a
		      WHERE a.user_id = u.id
		        AND a.action = 'auth.login_success'
		        AND a.created_at > NOW() - INTERVAL '30 days'
		    )
		  )                                                           AS inactive_30d
		FROM users u
		WHERE u.is_active = true
	`).Scan(&s.TotalUsers, &s.UsersWithoutMFA, &s.UsersInactive30d)
	return s, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

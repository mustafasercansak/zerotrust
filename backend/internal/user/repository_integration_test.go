package user

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/pkg/database"
)

type expandingEncrypter struct{}

func (expandingEncrypter) EncryptData(_ context.Context, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	return "vault:v1:" + strings.Repeat("x", 300) + plaintext, nil
}

func (expandingEncrypter) DecryptData(_ context.Context, ciphertext string) (string, error) {
	if ciphertext == "" || !strings.HasPrefix(ciphertext, "vault:v1:") {
		return ciphertext, nil
	}
	return strings.TrimPrefix(ciphertext, "vault:v1:"+strings.Repeat("x", 300)), nil
}

func setupUserRepository(t *testing.T) (*Repository, *pgxpool.Pool, context.Context) {
	t.Helper()
	dbURL := testdb.URL(t)

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test db unreachable: %v", err)
	}
	if err := database.RunMigrations(dbURL, "../../migrations"); err != nil {
		pool.Close()
		t.Fatalf("migrations failed: %v", err)
	}

	if _, err := pool.Exec(ctx, "TRUNCATE TABLE users CASCADE"); err != nil {
		pool.Close()
		t.Fatalf("cleanup failed: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO roles (name, description) VALUES ('admin', 'Admin role')
		ON CONFLICT (name) DO NOTHING
	`); err != nil {
		pool.Close()
		t.Fatalf("seed admin role failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO permissions (resource, action) VALUES ('users', 'read')
		ON CONFLICT (resource, action) DO NOTHING
	`); err != nil {
		pool.Close()
		t.Fatalf("seed users:read permission failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO role_permissions (role_id, permission_id)
		SELECT r.id, p.id
		FROM roles r, permissions p
		WHERE r.name = 'admin' AND p.resource = 'users' AND p.action = 'read'
		ON CONFLICT DO NOTHING
	`); err != nil {
		pool.Close()
		t.Fatalf("seed role permissions failed: %v", err)
	}

	return NewRepository(pool), pool, ctx
}

func TestRepositoryCreateAndFinders(t *testing.T) {
	repo, pool, ctx := setupUserRepository(t)
	defer pool.Close()

	u, err := repo.Create(ctx, "alpha@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if u.Email != "alpha@example.com" {
		t.Fatalf("email=%q want=alpha@example.com", u.Email)
	}

	byID, err := repo.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("find by id failed: %v", err)
	}
	if byID.ID != u.ID {
		t.Fatalf("find by id=%q want=%q", byID.ID, u.ID)
	}

	byEmail, err := repo.FindByEmail(ctx, "alpha@example.com")
	if err != nil {
		t.Fatalf("find by email failed: %v", err)
	}
	if byEmail.ID != u.ID {
		t.Fatalf("find by email id=%q want=%q", byEmail.ID, u.ID)
	}

	_, err = repo.Create(ctx, "alpha@example.com", "hash2", "tr")
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate create err=%v want=%v", err, ErrEmailTaken)
	}

	_, err = repo.FindByID(ctx, "00000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("find missing id err=%v want=%v", err, ErrNotFound)
	}

	_, err = repo.FindByEmail(ctx, "missing@example.com")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("find missing email err=%v want=%v", err, ErrNotFound)
	}
}

func TestRepositoryRoleMutationsAndRollback(t *testing.T) {
	repo, pool, ctx := setupUserRepository(t)
	defer pool.Close()

	u, err := repo.CreateWithRoles(ctx, "roles@example.com", "hash", "en", []string{"admin"})
	if err != nil {
		t.Fatalf("create with roles failed: %v", err)
	}
	if len(u.Roles) != 1 || u.Roles[0] != "admin" {
		t.Fatalf("roles=%v want [admin]", u.Roles)
	}

	err = repo.SetRoles(ctx, u.ID, []string{"unknown-role"})
	if !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("set roles err=%v want=%v", err, ErrUnknownRole)
	}

	got, err := repo.FindByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("find by id after rollback failed: %v", err)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "admin" {
		t.Fatalf("roles after rollback=%v want [admin]", got.Roles)
	}

	err = repo.AssignRoleByName(ctx, u.ID, "admin")
	if err != nil {
		t.Fatalf("assign role first call failed: %v", err)
	}
	err = repo.AssignRoleByName(ctx, u.ID, "admin")
	if err != nil {
		t.Fatalf("assign role second call failed: %v", err)
	}

	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1::uuid AND r.name = 'admin'
	`, u.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count admin role assignments failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("admin role assignment count=%d want=1", count)
	}
}

func TestRepositoryMutationsAndListFilters(t *testing.T) {
	repo, pool, ctx := setupUserRepository(t)
	defer pool.Close()

	u1, err := repo.Create(ctx, "alpha@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create u1 failed: %v", err)
	}
	u2, err := repo.Create(ctx, "beta@example.com", "hash", "tr")
	if err != nil {
		t.Fatalf("create u2 failed: %v", err)
	}

	if err := repo.UpdateLocale(ctx, u1.ID, "fr"); err != nil {
		t.Fatalf("update locale failed: %v", err)
	}
	updated, err := repo.UpdateProfile(ctx, u1.ID, "Jane", "Doe")
	if err != nil {
		t.Fatalf("update profile failed: %v", err)
	}
	if updated.FirstName != "Jane" || updated.LastName != "Doe" {
		t.Fatalf("updated profile mismatch: %+v", updated)
	}

	if err := repo.UpdatePassword(ctx, u1.ID, "new-hash"); err != nil {
		t.Fatalf("update password failed: %v", err)
	}
	byEmail, err := repo.FindByEmail(ctx, "alpha@example.com")
	if err != nil {
		t.Fatalf("find by email after update failed: %v", err)
	}
	if byEmail.PasswordHash != "new-hash" {
		t.Fatalf("password hash=%q want=new-hash", byEmail.PasswordHash)
	}

	avatarUser, err := repo.UpdateAvatar(ctx, u1.ID, "avatar-key", 123)
	if err != nil {
		t.Fatalf("update avatar failed: %v", err)
	}
	if !avatarUser.HasAvatar {
		t.Fatal("expected HasAvatar=true after setting avatar key")
	}

	if err := repo.SetActive(ctx, u2.ID, false); err != nil {
		t.Fatalf("set active failed: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, expires_at, is_revoked, last_used_at)
		VALUES ($1::uuid, 'active-session-hash', NOW() + INTERVAL '1 hour', false, NOW())
	`, u1.ID); err != nil {
		t.Fatalf("insert session failed: %v", err)
	}

	resActive, err := repo.List(ctx, ListParams{Limit: 10, SortBy: "email", SortDir: "asc", Status: "active"})
	if err != nil {
		t.Fatalf("list active failed: %v", err)
	}
	if resActive.Total != 1 {
		t.Fatalf("active total=%d want=1", resActive.Total)
	}
	if got := resActive.ActiveSessions[u1.ID]; got != 1 {
		t.Fatalf("active session count=%d want=1", got)
	}

	resInactive, err := repo.List(ctx, ListParams{Limit: 10, Status: "inactive"})
	if err != nil {
		t.Fatalf("list inactive failed: %v", err)
	}
	if resInactive.Total != 1 {
		t.Fatalf("inactive total=%d want=1", resInactive.Total)
	}

	resEmail, err := repo.List(ctx, ListParams{Limit: 10, Email: "alpha@example.com"})
	if err != nil {
		t.Fatalf("list email filter failed: %v", err)
	}
	if resEmail.Total != 1 {
		t.Fatalf("email filtered total=%d want=1", resEmail.Total)
	}

	resFallback, err := repo.List(ctx, ListParams{Limit: 999, SortBy: "nope", SortDir: "asc"})
	if err != nil {
		t.Fatalf("list fallback failed: %v", err)
	}
	if resFallback.Total != 2 {
		t.Fatalf("fallback total=%d want=2", resFallback.Total)
	}

	if _, err := repo.UpdateProfile(ctx, "00000000-0000-0000-0000-000000000000", "x", "y"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update profile missing err=%v want=%v", err, ErrNotFound)
	}
	if _, err := repo.UpdateAvatar(ctx, "00000000-0000-0000-0000-000000000000", "k", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update avatar missing err=%v want=%v", err, ErrNotFound)
	}
	if err := repo.SetActive(ctx, "00000000-0000-0000-0000-000000000000", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("set active missing err=%v want=%v", err, ErrNotFound)
	}
}

func TestRepositoryEncryptedFieldsAllowExpandedCiphertext(t *testing.T) {
	repo, pool, ctx := setupUserRepository(t)
	defer pool.Close()
	repo.SetSecretsClient(expandingEncrypter{})

	u, err := repo.Create(ctx, "encrypted@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create encrypted user failed: %v", err)
	}
	if u.Email != "encrypted@example.com" {
		t.Fatalf("email=%q want=encrypted@example.com", u.Email)
	}

	firstName := strings.Repeat("A", 80)
	lastName := strings.Repeat("B", 80)
	updated, err := repo.UpdateProfile(ctx, u.ID, firstName, lastName)
	if err != nil {
		t.Fatalf("update encrypted profile failed: %v", err)
	}
	if updated.FirstName != firstName || updated.LastName != lastName {
		t.Fatalf("profile mismatch: first=%q last=%q", updated.FirstName, updated.LastName)
	}

	var storedEmail, storedFirstName, storedLastName string
	if err := pool.QueryRow(ctx, `
		SELECT email, first_name, last_name
		FROM users
		WHERE id = $1::uuid
	`, u.ID).Scan(&storedEmail, &storedFirstName, &storedLastName); err != nil {
		t.Fatalf("read encrypted fields failed: %v", err)
	}
	if len(storedEmail) <= 255 {
		t.Fatalf("stored email ciphertext length=%d want >255", len(storedEmail))
	}
	if len(storedFirstName) <= 80 || len(storedLastName) <= 80 {
		t.Fatalf("stored profile ciphertext lengths=(%d,%d) want >80", len(storedFirstName), len(storedLastName))
	}
}

func TestRepositoryGetPermissions(t *testing.T) {
	repo, pool, ctx := setupUserRepository(t)
	defer pool.Close()

	u, err := repo.Create(ctx, "perms@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if err := repo.AssignRoleByName(ctx, u.ID, "admin"); err != nil {
		t.Fatalf("assign admin role failed: %v", err)
	}

	perms, err := repo.GetPermissions(ctx, u.ID)
	if err != nil {
		t.Fatalf("get permissions failed: %v", err)
	}
	if len(perms) == 0 {
		t.Fatal("expected non-empty permissions for admin role")
	}

	hasUsersRead := false
	for _, p := range perms {
		if p == "users:read" {
			hasUsersRead = true
			break
		}
	}
	if !hasUsersRead {
		t.Fatalf("users:read not found in permissions: %v", perms)
	}
}

func TestRepositoryCreateWithRolesUnknownRole(t *testing.T) {
	repo, pool, ctx := setupUserRepository(t)
	defer pool.Close()

	_, err := repo.CreateWithRoles(ctx, "badrole@example.com", "hash", "en", []string{"admin", "unknown"})
	if !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("create with unknown role err=%v want=%v", err, ErrUnknownRole)
	}

	_, err = repo.FindByEmail(ctx, "badrole@example.com")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("user should not be created on role failure, err=%v want=%v", err, ErrNotFound)
	}
}

func TestRepositoryListActiveSessionWindow(t *testing.T) {
	repo, pool, ctx := setupUserRepository(t)
	defer pool.Close()

	u, err := repo.Create(ctx, "window@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, expires_at, is_revoked, last_used_at)
		VALUES ($1::uuid, 'old-session', NOW() + INTERVAL '1 hour', false, NOW() - INTERVAL '5 minutes')
	`, u.ID); err != nil {
		t.Fatalf("insert old session failed: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, expires_at, is_revoked, last_used_at)
		VALUES ($1::uuid, 'fresh-session', NOW() + INTERVAL '1 hour', false, NOW())
	`, u.ID); err != nil {
		t.Fatalf("insert fresh session failed: %v", err)
	}

	res, err := repo.List(ctx, ListParams{Limit: 10})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if got := res.ActiveSessions[u.ID]; got != 1 {
		t.Fatalf("active sessions in window=%d want=1", got)
	}
}

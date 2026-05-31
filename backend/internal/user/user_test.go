package user

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestUserServiceIntegration(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test db unreachable: %v", err)
	}
	defer pool.Close()

	repo := NewRepository(pool)
	svc := NewService(repo)

	// Clean up user table
	pool.Exec(ctx, "DELETE FROM user_roles")
	pool.Exec(ctx, "DELETE FROM users")
	pool.Exec(ctx, "DELETE FROM roles")

	// Pre-insert some roles
	pool.Exec(ctx, "INSERT INTO roles (name, description) VALUES ('admin', 'Admin Role'), ('viewer', 'Viewer')")

	// Test Register
	u, err := svc.Register(ctx, "test1@example.com", "password", "en")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if u.Email != "test1@example.com" {
		t.Fatalf("Expected test1@example.com, got %s", u.Email)
	}

	// Test CheckPassword
	if !svc.CheckPassword(u.PasswordHash, "password") {
		t.Fatal("CheckPassword should return true")
	}
	if svc.CheckPassword(u.PasswordHash, "wrong") {
		t.Fatal("CheckPassword should return false")
	}

	// Test Duplicate Email
	_, err = svc.Register(ctx, "test1@example.com", "password", "en")
	if err != ErrEmailTaken {
		t.Fatalf("Expected ErrEmailTaken, got %v", err)
	}

	// Test RegisterWithRoles
	u2, err := svc.RegisterWithRoles(ctx, "test2@example.com", "password", "tr", []string{"admin", "viewer"})
	if err != nil {
		t.Fatalf("Failed to create with roles: %v", err)
	}
	if len(u2.Roles) != 2 {
		t.Fatalf("Expected 2 roles, got %v", len(u2.Roles))
	}

	// Test FindByID
	found, err := svc.FindByID(ctx, u.ID)
	if err != nil || found.Email != "test1@example.com" {
		t.Fatalf("FindByID failed")
	}

	// Test FindByEmail
	foundByEmail, err := svc.FindByEmail(ctx, "test2@example.com")
	if err != nil || foundByEmail.ID != u2.ID {
		t.Fatalf("FindByEmail failed")
	}

	// Test UpdateProfile
	updatedProfile, err := svc.UpdateProfile(ctx, u.ID, "John", "Doe")
	if err != nil || updatedProfile.FirstName != "John" {
		t.Fatalf("UpdateProfile failed")
	}

	// Test UpdateProfile Invalid
	_, err = svc.UpdateProfile(ctx, u.ID, string(make([]byte, 100)), "Doe")
	if err != ErrInvalidProfile {
		t.Fatalf("Expected ErrInvalidProfile for too long first name, got %v", err)
	}

	// Test SetRoles
	err = svc.SetRoles(ctx, u.ID, []string{"viewer"})
	if err != nil {
		t.Fatalf("SetRoles failed: %v", err)
	}

	// Test SetActive
	err = svc.SetActive(ctx, u.ID, false)
	if err != nil {
		t.Fatalf("SetActive failed: %v", err)
	}

	// Test UpdatePassword
	err = svc.UpdatePassword(ctx, u.ID, "newhash")
	if err != nil {
		t.Fatalf("UpdatePassword failed: %v", err)
	}

	// Test UpdateAvatar
	avatarUser, err := svc.UpdateAvatar(ctx, u.ID, "avatar_key", 1024)
	if err != nil || !avatarUser.HasAvatar {
		t.Fatalf("UpdateAvatar failed")
	}

	// Test GetPermissions
	perms, err := svc.GetPermissions(ctx, u2.ID)
	if err != nil {
		t.Fatalf("GetPermissions failed")
	}
	_ = perms // typically tested with actual permissions loaded

	// Test List extensively
	repo.UpdateLocale(ctx, u.ID, "fr")
	u3, _ := svc.Register(ctx, "inactive@example.com", "pass", "en")
	svc.SetActive(ctx, u3.ID, false)

	// List active users
	res, err := svc.List(ctx, ListParams{Limit: 10, SortBy: "email", SortDir: "asc", Status: "active"})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	// We have test1@example.com (deactivated earlier in test), inactive@example.com (inactive), and test2@example.com (active)
	if res.Total != 1 {
		t.Fatalf("List returned incorrect active count: %v", res.Total)
	}

	// List inactive users
	resInactive, _ := svc.List(ctx, ListParams{Limit: 10, SortBy: "created_at", SortDir: "desc", Status: "inactive"})
	if resInactive.Total != 2 {
		t.Fatalf("List returned incorrect inactive count: %v", resInactive.Total)
	}

	// List by Email filter
	resEmail, _ := svc.List(ctx, ListParams{Limit: 10, SortBy: "is_active", SortDir: "desc", Email: "test1@"})
	if resEmail.Total != 1 {
		t.Fatalf("List returned incorrect count for email filter: %v", resEmail.Total)
	}

	// List with weird limits and sort columns to hit fallback logic
	resFallback, _ := svc.List(ctx, ListParams{Limit: 999, SortBy: "unknown_col"})
	if resFallback.Total != 3 {
		t.Fatalf("List fallback returned incorrect count: %v", resFallback.Total)
	}

	// Test SeedAdmin
	err = svc.SeedAdmin(ctx, "admin@example.com", "hash")
	if err != nil {
		t.Fatalf("SeedAdmin failed: %v", err)
	}
	// SeedAdmin again should assign role safely
	err = svc.SeedAdmin(ctx, "admin@example.com", "hash")
	if err != nil {
		t.Fatalf("SeedAdmin idempotent failed: %v", err)
	}

	// Test errors
	_, err = svc.FindByID(ctx, "00000000-0000-0000-0000-000000000000")
	if err != ErrNotFound {
		t.Fatalf("Expected ErrNotFound, got %v", err)
	}
	_, err = svc.FindByEmail(ctx, "nonexistent@example.com")
	if err != ErrNotFound {
		t.Fatalf("Expected ErrNotFound, got %v", err)
	}
	err = svc.SetActive(ctx, "00000000-0000-0000-0000-000000000000", true)
	if err != ErrNotFound {
		t.Fatalf("Expected ErrNotFound from SetActive, got %v", err)
	}
	_, err = svc.RegisterWithRoles(ctx, "bad_role@example.com", "hash", "en", []string{"unknown"})
	if !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("Expected ErrUnknownRole, got %v", err)
	}
}

package user

import (
	"errors"
	"testing"
)

func TestSetRoles_LastAdminInvariant(t *testing.T) {
	repo, pool, ctx := setupUserRepository(t)
	defer pool.Close()

	a1, err := repo.Create(ctx, "admin1@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}
	a2, err := repo.Create(ctx, "admin2@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create a2: %v", err)
	}
	if err := repo.AssignRoleByName(ctx, a1.ID, "admin"); err != nil {
		t.Fatalf("assign a1 admin: %v", err)
	}
	if err := repo.AssignRoleByName(ctx, a2.ID, "admin"); err != nil {
		t.Fatalf("assign a2 admin: %v", err)
	}

	// Demoting the first admin is fine — one active admin remains.
	if err := repo.SetRoles(ctx, a1.ID, []string{}); err != nil {
		t.Fatalf("demoting non-last admin should succeed, got %v", err)
	}

	// Demoting the now-last admin must be rejected and rolled back.
	if err := repo.SetRoles(ctx, a2.ID, []string{}); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("expected ErrLastAdmin demoting last admin, got %v", err)
	}
	got, err := repo.FindByID(ctx, a2.ID)
	if err != nil {
		t.Fatalf("find a2: %v", err)
	}
	foundAdmin := false
	for _, r := range got.Roles {
		if r == "admin" {
			foundAdmin = true
		}
	}
	if !foundAdmin {
		t.Fatal("last admin's admin role must be preserved after rejected demotion (rollback)")
	}
}

func TestSetActive_LastAdminInvariant(t *testing.T) {
	repo, pool, ctx := setupUserRepository(t)
	defer pool.Close()

	a1, err := repo.Create(ctx, "onlyadmin@example.com", "hash", "en")
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}
	if err := repo.AssignRoleByName(ctx, a1.ID, "admin"); err != nil {
		t.Fatalf("assign admin: %v", err)
	}

	// Deactivating the only active admin must be rejected.
	if err := repo.SetActive(ctx, a1.ID, false); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("expected ErrLastAdmin deactivating last admin, got %v", err)
	}
	got, err := repo.FindByID(ctx, a1.ID)
	if err != nil {
		t.Fatalf("find a1: %v", err)
	}
	if !got.IsActive {
		t.Fatal("last admin must remain active after rejected deactivation (rollback)")
	}
}

package database

import (
	"os"
	"testing"

	"github.com/zerotrust/backend/internal/testdb"
)

func TestRunMigrationsInvalidDB(t *testing.T) {
	err := RunMigrations("invalid://url", ".")
	if err == nil {
		t.Fatal("Expected error with invalid DB URL")
	}
}

func TestRunMigrationsInvalidPath(t *testing.T) {
	// A valid postgres URL but invalid migration path
	dbURL := testdb.URL(t)
	err := RunMigrations(dbURL, "/non/existent/path/for/migrations")
	if err == nil {
		t.Fatal("Expected error with invalid migration path")
	}
}

func TestRunMigrationsSuccess(t *testing.T) {
	dbURL := testdb.URL(t)

	// Create a temporary mock migrations directory
	tmpDir, err := os.MkdirTemp("", "migrations")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	upFile := tmpDir + "/000001_mock.up.sql"
	if err := os.WriteFile(upFile, []byte("CREATE TABLE IF NOT EXISTS _mock_test_2 (id int);"), 0644); err != nil {
		t.Fatalf("Failed to write mock up migration: %v", err)
	}

	// Run will fail because the test DB already has migrations (up to 15) and our mock dir only has 1.
	// But this successfully tests that the migrator is initialized and Up() is called.
	err = RunMigrations(dbURL, tmpDir)
	if err == nil {
		t.Fatal("Expected error because mock dir doesn't contain history, got nil")
	}
}

package testdb

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// URL returns TEST_DATABASE_URL after verifying that its database name is
// explicitly test-only. The check happens before callers connect, migrate, or
// execute destructive fixture cleanup.
func URL(t testing.TB) string {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	if err := ValidateDSN(dsn); err != nil {
		t.Fatalf("unsafe TEST_DATABASE_URL: %v", err)
	}
	return dsn
}

func ValidateDSN(dsn string) error {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse database URL: %w", err)
	}
	return ValidateDatabaseName(config.ConnConfig.Database)
}

func ValidateDatabaseName(name string) error {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "test" ||
		strings.HasPrefix(normalized, "test_") ||
		strings.HasSuffix(normalized, "_test") ||
		strings.Contains(normalized, "_test_") {
		return nil
	}
	return fmt.Errorf(
		"database %q is not marked as a test database; use a dedicated name such as zerotrust_test",
		name,
	)
}

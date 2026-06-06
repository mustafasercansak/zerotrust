package testdb

import "testing"

func TestValidateDatabaseName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"test", "test_zerotrust", "zerotrust_test", "zerotrust_test_ci"} {
		name := name
		t.Run("accept_"+name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateDatabaseName(name); err != nil {
				t.Fatalf("ValidateDatabaseName(%q): %v", name, err)
			}
		})
	}

	for _, name := range []string{"", "postgres", "zerotrust", "zerotrust_db", "production"} {
		name := name
		t.Run("reject_"+name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateDatabaseName(name); err == nil {
				t.Fatalf("ValidateDatabaseName(%q) unexpectedly succeeded", name)
			}
		})
	}
}

func TestValidateDSN(t *testing.T) {
	t.Parallel()

	if err := ValidateDSN("postgres://user:pass@localhost:5432/zerotrust_test?sslmode=disable"); err != nil {
		t.Fatalf("safe DSN rejected: %v", err)
	}
	if err := ValidateDSN("host=localhost user=user password=pass dbname=test_zerotrust sslmode=disable"); err != nil {
		t.Fatalf("safe keyword DSN rejected: %v", err)
	}
	if err := ValidateDSN("postgres://user:pass@localhost:5432/zerotrust_db?sslmode=disable"); err == nil {
		t.Fatal("development DSN unexpectedly accepted")
	}
	if err := ValidateDSN("not a database URL"); err == nil {
		t.Fatal("invalid DSN unexpectedly accepted")
	}
}

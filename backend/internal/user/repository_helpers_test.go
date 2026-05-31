package user

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsUniqueViolation(t *testing.T) {
	t.Run("true for postgres unique violation code 23505", func(t *testing.T) {
		err := &pgconn.PgError{Code: "23505"}
		if !isUniqueViolation(err) {
			t.Fatal("expected true for unique violation")
		}
	})

	t.Run("false for non-unique postgres errors", func(t *testing.T) {
		err := &pgconn.PgError{Code: "23503"}
		if isUniqueViolation(err) {
			t.Fatal("expected false for non-unique postgres error")
		}
	})

	t.Run("false for non-postgres errors", func(t *testing.T) {
		if isUniqueViolation(errors.New("boom")) {
			t.Fatal("expected false for generic error")
		}
	})
}

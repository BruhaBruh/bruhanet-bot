package store

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testStore открывает Store в отдельной схеме t_<rand>; без TEST_DATABASE_URL тест пропускается.
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	schema := "t_" + hex.EncodeToString(b)

	admin, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	if _, err = admin.Exec(t.Context(), `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(t.Context(), `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})

	s, err := open(t.Context(), url, func(c *pgxpool.Config) {
		c.ConnConfig.RuntimeParams["search_path"] = schema
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(s.Close)

	if err = s.Migrate(t.Context()); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	return s
}

func TestMigrateIdempotent(t *testing.T) {
	s := testStore(t)
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var n int
	_ = s.pool.QueryRow(t.Context(), `SELECT count(*) FROM payments`).Scan(&n)
	if n != 0 {
		t.Fatalf("payments = %d", n)
	}
}

package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testStore открывает Store в отдельной схеме t_<rand> и накатывает миграции.
func testStore(t *testing.T) *Store {
	t.Helper()
	s := testStoreRaw(t)
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	return s
}

// testStoreRaw — то же без Migrate; без TEST_DATABASE_URL тест пропускается.
func testStoreRaw(t *testing.T) *Store {
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
		// t.Context() к этому моменту уже отменён.
		if _, err := admin.Exec(context.WithoutCancel(t.Context()), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		admin.Close()
	})

	s, err := open(t.Context(), url, Option(func(c *pgxpool.Config) {
		c.ConnConfig.RuntimeParams["search_path"] = schema
	}))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestCheckSchema(t *testing.T) {
	s := testStoreRaw(t)
	if err := s.CheckSchema(t.Context()); !errors.Is(err, ErrSchemaOutdated) {
		t.Fatalf("fresh schema: want ErrSchemaOutdated, got %v", err)
	}
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.CheckSchema(t.Context()); err != nil {
		t.Fatalf("migrated schema: %v", err)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	s := testStore(t)
	if err := s.Migrate(t.Context()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var n int
	if err := s.pool.QueryRow(t.Context(), `SELECT count(*) FROM payments`).Scan(&n); err != nil {
		t.Fatalf("count payments: %v", err)
	}
	if n != 0 {
		t.Fatalf("payments = %d", n)
	}
}

func TestOnePendingPerUser(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	if _, err := s.pool.Exec(ctx, `INSERT INTO users (telegram_id, username) VALUES (1, 'u')`); err != nil {
		t.Fatal(err)
	}
	const ins = `INSERT INTO payments (telegram_id, username, months, price, status, prev_expire_at)
		VALUES (1, 'u', 1, 150, $1, now())`

	if _, err := s.pool.Exec(ctx, ins, "cancelled"); err != nil {
		t.Fatalf("cancelled: %v", err)
	}
	if _, err := s.pool.Exec(ctx, ins, "pending"); err != nil {
		t.Fatalf("first pending: %v", err)
	}
	_, err := s.pool.Exec(ctx, ins, "pending")
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23505" {
		t.Fatalf("second pending: want unique violation, got %v", err)
	}
}

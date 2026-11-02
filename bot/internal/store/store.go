package store

import (
	"context"
	"embed"
	"errors"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/bruhabruh/bruhanet/bot/internal/observability"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	pool *pgxpool.Pool
}

// Open подключается к Postgres с трассировкой запросов и проверяет соединение.
func Open(ctx context.Context, url string) (*Store, error) {
	return open(ctx, url)
}

func open(ctx context.Context, url string, opts ...func(*pgxpool.Config)) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.Join(ErrOpen, err)
	}
	cfg.ConnConfig.Tracer = observability.PostgresTracer()
	for _, o := range opts {
		o(cfg)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.Join(ErrOpen, err)
	}
	if err = pool.Ping(observability.WithProbe(ctx)); err != nil {
		pool.Close()
		return nil, errors.Join(ErrOpen, err)
	}
	return &Store{pool: pool}, nil
}

// Migrate накатывает встроенные goose-миграции.
func (s *Store) Migrate(ctx context.Context) error {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return errors.Join(ErrMigrate, err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, stdlib.OpenDBFromPool(s.pool), sub)
	if err != nil {
		return errors.Join(ErrMigrate, err)
	}
	if _, err = p.Up(ctx); err != nil {
		return errors.Join(ErrMigrate, err)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) Close() { s.pool.Close() }

// ponytail: пока без вызовов, используется методами платежей следующих задач.
//
//nolint:unused
func (s *Store) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, fn)
}

package store

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	pool *pgxpool.Pool

	claimMu    sync.Mutex
	claimFails map[uuid.UUID]int // сбои запуска запланированных рассылок (ClaimDue), в памяти процесса
}

// Option настраивает пул до подключения.
type Option func(*pgxpool.Config)

// WithTracer включает трассировку запросов.
func WithTracer(t pgx.QueryTracer) Option {
	return func(c *pgxpool.Config) { c.ConnConfig.Tracer = t }
}

// Open подключается к Postgres и проверяет соединение (Ping — переданным ctx).
func Open(ctx context.Context, url string, opts ...Option) (*Store, error) {
	return open(ctx, url, opts...)
}

func open(ctx context.Context, url string, opts ...Option) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.Join(ErrOpen, err)
	}
	for _, o := range opts {
		o(cfg)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.Join(ErrOpen, err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.Join(ErrOpen, err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) provider() (*goose.Provider, error) {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, stdlib.OpenDBFromPool(s.pool), sub)
}

// Migrate накатывает встроенные goose-миграции.
func (s *Store) Migrate(ctx context.Context) error {
	p, err := s.provider()
	if err != nil {
		return errors.Join(ErrMigrate, err)
	}
	if _, err = p.Up(ctx); err != nil {
		return errors.Join(ErrMigrate, err)
	}
	return nil
}

// CheckSchema возвращает ErrSchemaOutdated, если есть непримененные миграции.
func (s *Store) CheckSchema(ctx context.Context) error {
	p, err := s.provider()
	if err != nil {
		return errors.Join(ErrMigrate, err)
	}
	pending, err := p.HasPending(ctx)
	if err != nil {
		return errors.Join(ErrMigrate, err)
	}
	if pending {
		return ErrSchemaOutdated
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) Close() { s.pool.Close() }

func (s *Store) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, fn)
}

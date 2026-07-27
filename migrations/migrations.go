package migrations

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed *.sql
var sqlFiles embed.FS

func Up(ctx context.Context, db *sql.DB) ([]*goose.MigrationResult, error) {
	p, err := newLockingProvider(db)
	if err != nil {
		return nil, err
	}
	results, err := p.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return results, nil
}

func Down(ctx context.Context, db *sql.DB) (*goose.MigrationResult, error) {
	p, err := newLockingProvider(db)
	if err != nil {
		return nil, err
	}
	result, err := p.Down(ctx)
	if err != nil {
		return nil, fmt.Errorf("roll back migration: %w", err)
	}
	return result, nil
}

func Status(ctx context.Context, db *sql.DB) ([]*goose.MigrationStatus, error) {
	p, err := newProvider(db)
	if err != nil {
		return nil, err
	}
	return p.Status(ctx)
}

func Verify(ctx context.Context, dsn string) error {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse pg config: %w", err)
	}
	db := stdlib.OpenDB(*cfg)
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	p, err := newProvider(db)
	if err != nil {
		return err
	}
	pending, err := p.HasPending(ctx)
	if err != nil {
		return fmt.Errorf("check pending migrations: %w", err)
	}
	if pending {
		return errors.New("schema outdated: run migrations")
	}
	return nil
}

func newProvider(db *sql.DB, opts ...goose.ProviderOption) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectPostgres, db, sqlFiles, opts...)
}

func newLockingProvider(db *sql.DB) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("create session locker: %w", err)
	}
	return newProvider(db, goose.WithSessionLocker(locker))
}

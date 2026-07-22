package store

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// A SQLSTATE code is 5 characters; its first 2 identify the error class.
	sqlStateLen      = 5
	sqlStateClassLen = 2
	// SQLSTATE classes that signal the database is temporarily unavailable.
	classConnectionException   = "08"
	classInsufficientResources = "53"
	classOperatorIntervention  = "57"
	classSystemError           = "58"
	// SQLSTATEs of a blocked delete
	codeRestrictViolation   = "23001"
	codeForeignKeyViolation = "23503"
)

type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres wraps an open connection pool in a Postgres store.
func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return new(Postgres{pool: pool})
}

func (pg *Postgres) Ping(ctx context.Context) error {
	return pg.pool.Ping(ctx)
}

// withTx wraps fn in a transaction and maps its error so callers never leak a raw DB error.
func (pg *Postgres) withTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := pg.pool.Begin(ctx)
	if err != nil {
		return mapDBError(err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return mapDBError(err)
	}
	return mapDBError(tx.Commit(ctx))
}

// mapDBError tags connection-class failures as domain.ErrUnavailable.
func mapDBError(err error) error {
	if err == nil ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		if code := pgErr.Code; len(code) == sqlStateLen {
			switch code[:sqlStateClassLen] {
			case classConnectionException, classInsufficientResources,
				classOperatorIntervention, classSystemError:
				return fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
			}
		}
		return err
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
	}
	return err
}

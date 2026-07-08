package store

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
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

func (pg *Postgres) Save(
	ctx context.Context, p domain.Product,
) (domain.Product, error) {
	row := pg.pool.QueryRow(
		ctx, queryInsert, p.ID, p.Name, p.Price.MinorAmount, string(p.Price.Currency), p.Stock,
	)
	if err := row.Scan(&p.Version); err != nil {
		return domain.Product{}, mapDBError(err)
	}
	return p, nil
}

func (pg *Postgres) FindByID(
	ctx context.Context, id uuid.UUID,
) (domain.Product, error) {
	row := pg.pool.QueryRow(ctx, queryGetByID, id)

	var p domain.Product
	var currency string
	if err := row.Scan(
		&p.ID, &p.Name, &p.Price.MinorAmount, &currency, &p.Stock, &p.Version,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Product{}, domain.ErrNotFound
		}
		return domain.Product{}, mapDBError(err)
	}
	p.Price.Currency = domain.Currency(currency)
	return p, nil
}

func (pg *Postgres) FindAll(
	ctx context.Context, cursor uuid.NullUUID, limit int,
) (domain.ProductPage, error) {
	var (
		rows      pgx.Rows
		err       error
		pageLimit = limit + 1
	)

	if cursor.Valid {
		rows, err = pg.pool.Query(ctx, queryGetAllAfterCursor, cursor.UUID, pageLimit)
	} else {
		rows, err = pg.pool.Query(ctx, queryGetAll, pageLimit)
	}
	if err != nil {
		return domain.ProductPage{}, mapDBError(err)
	}
	defer rows.Close()

	products := make([]domain.Product, 0, limit+1)
	for rows.Next() {
		var p domain.Product
		var currency string
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Price.MinorAmount, &currency, &p.Stock, &p.Version,
		); err != nil {
			return domain.ProductPage{}, mapDBError(err)
		}
		p.Price.Currency = domain.Currency(currency)
		products = append(products, p)
	}

	if err = rows.Err(); err != nil {
		return domain.ProductPage{}, mapDBError(err)
	}

	return productPage(products, limit), nil
}

func (pg *Postgres) Update(
	ctx context.Context, p domain.Product,
) (domain.Product, error) {
	var currency string
	err := pg.pool.QueryRow(
		ctx, queryUpdate, p.ID, p.Name, p.Price.MinorAmount, string(p.Price.Currency),
	).Scan(&p.ID, &p.Name, &p.Price.MinorAmount, &currency, &p.Stock, &p.Version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Product{}, domain.ErrNotFound
		}
		return domain.Product{}, mapDBError(err)
	}
	p.Price.Currency = domain.Currency(currency)
	return p, nil
}

func (pg *Postgres) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := pg.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return mapDBError(err)
	}
	if res.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Purchase atomically decrements stock in an explicit tx.
func (pg *Postgres) Purchase(
	ctx context.Context, id uuid.UUID, qty int64,
) (domain.Product, error) {
	tx, err := pg.pool.Begin(ctx)
	if err != nil {
		return domain.Product{}, mapDBError(err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var p domain.Product
	var currency string
	if err := tx.QueryRow(ctx, queryPurchase, id, qty).Scan(
		&p.ID, &p.Name, &p.Price.MinorAmount, &currency, &p.Stock, &p.Version,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Product{}, purchaseNoRowError(ctx, tx, id)
		}
		return domain.Product{}, mapDBError(err)
	}

	p.Price.Currency = domain.Currency(currency)

	if err := tx.Commit(ctx); err != nil {
		return domain.Product{}, mapDBError(err)
	}
	return p, nil
}

// purchaseNoRowError tells a missing product apart from insufficient stock.
func purchaseNoRowError(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var exists bool
	if err := tx.QueryRow(ctx, queryProductExists, id).Scan(&exists); err != nil {
		return mapDBError(err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	return domain.ErrInsufficientStock
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

func productPage(products []domain.Product, limit int) domain.ProductPage {
	if len(products) <= limit {
		return domain.ProductPage{Items: products}
	}

	return domain.ProductPage{
		Items:   products[:limit],
		HasMore: true,
	}
}

package repository

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/alkmc/storefront/internal/entity"
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

type Repository struct {
	pool *pgxpool.Pool
}

// New wraps an open connection pool in a repository.
func New(pool *pgxpool.Pool) *Repository {
	return new(Repository{pool: pool})
}

func (pg *Repository) Ping(ctx context.Context) error {
	return pg.pool.Ping(ctx)
}

func (pg *Repository) Save(ctx context.Context, p entity.Product) (entity.Product, error) {
	if _, err := pg.pool.Exec(
		ctx, queryInsert, p.ID, p.Name, p.Price.MinorAmount, string(p.Price.Currency),
	); err != nil {
		return entity.Product{}, mapDBError(err)
	}
	return p, nil
}

func (pg *Repository) FindByID(ctx context.Context, id uuid.UUID) (entity.Product, error) {
	row := pg.pool.QueryRow(ctx, queryGetByID, id)

	var p entity.Product
	var currency string
	if err := row.Scan(&p.ID, &p.Name, &p.Price.MinorAmount, &currency); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entity.Product{}, entity.ErrNotFound
		}
		return entity.Product{}, mapDBError(err)
	}
	p.Price.Currency = entity.Currency(currency)
	return p, nil
}

func (pg *Repository) FindAll(ctx context.Context, cursor uuid.NullUUID, limit int,
) (entity.ProductPage, error) {
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
		return entity.ProductPage{}, mapDBError(err)
	}
	defer rows.Close()

	products := make([]entity.Product, 0, limit+1)
	for rows.Next() {
		var p entity.Product
		var currency string
		if err := rows.Scan(&p.ID, &p.Name, &p.Price.MinorAmount, &currency); err != nil {
			return entity.ProductPage{}, mapDBError(err)
		}
		p.Price.Currency = entity.Currency(currency)
		products = append(products, p)
	}

	if err = rows.Err(); err != nil {
		return entity.ProductPage{}, mapDBError(err)
	}

	return productPage(products, limit), nil
}

func (pg *Repository) Update(ctx context.Context, p entity.Product) error {
	res, err := pg.pool.Exec(ctx, queryUpdate, p.ID, p.Name, p.Price.MinorAmount, string(p.Price.Currency))
	if err != nil {
		return mapDBError(err)
	}
	if res.RowsAffected() == 0 {
		return entity.ErrNotFound
	}
	return nil
}

func (pg *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := pg.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return mapDBError(err)
	}
	if res.RowsAffected() == 0 {
		return entity.ErrNotFound
	}
	return nil
}

// mapDBError tags connection-class failures as entity.ErrUnavailable.
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
				return fmt.Errorf("%w: %w", entity.ErrUnavailable, err)
			}
		}
		return err
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return fmt.Errorf("%w: %w", entity.ErrUnavailable, err)
	}
	return err
}

func productPage(products []entity.Product, limit int) entity.ProductPage {
	if len(products) <= limit {
		return entity.ProductPage{Items: products}
	}

	return entity.ProductPage{
		Items:   products[:limit],
		HasMore: true,
	}
}

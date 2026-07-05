package repository

import (
	"context"
	"errors"

	"github.com/alkmc/storefront/internal/entity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
		return entity.Product{}, err
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
		return entity.Product{}, err
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
		return entity.ProductPage{}, err
	}
	defer rows.Close()

	products := make([]entity.Product, 0, limit+1)
	for rows.Next() {
		var p entity.Product
		var currency string
		if err := rows.Scan(&p.ID, &p.Name, &p.Price.MinorAmount, &currency); err != nil {
			return entity.ProductPage{}, err
		}
		p.Price.Currency = entity.Currency(currency)
		products = append(products, p)
	}

	if err = rows.Err(); err != nil {
		return entity.ProductPage{}, err
	}

	return productPage(products, limit), nil
}

func (pg *Repository) Update(ctx context.Context, p entity.Product) error {
	res, err := pg.pool.Exec(ctx, queryUpdate, p.ID, p.Name, p.Price.MinorAmount, string(p.Price.Currency))
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return entity.ErrNotFound
	}
	return nil
}

func (pg *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := pg.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return entity.ErrNotFound
	}
	return nil
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

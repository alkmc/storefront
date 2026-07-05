package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/alkmc/storefront/internal/entity"
	"github.com/google/uuid"
)

type Repository struct {
	db *sql.DB
}

// New wraps an open database handle in a repository.
func New(db *sql.DB) *Repository {
	return new(Repository{db: db})
}

func (pg *Repository) Ping(ctx context.Context) error {
	return pg.db.PingContext(ctx)
}

func (pg *Repository) Save(ctx context.Context, p entity.Product) (entity.Product, error) {
	if _, err := pg.db.ExecContext(
		ctx, queryInsert, p.ID, p.Name, p.Price.MinorAmount, string(p.Price.Currency),
	); err != nil {
		return entity.Product{}, err
	}
	return p, nil
}

func (pg *Repository) FindByID(ctx context.Context, id uuid.UUID) (entity.Product, error) {
	row := pg.db.QueryRowContext(ctx, queryGetByID, id)

	var p entity.Product
	var currency string
	if err := row.Scan(&p.ID, &p.Name, &p.Price.MinorAmount, &currency); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
		rows      *sql.Rows
		err       error
		pageLimit = limit + 1
	)

	if cursor.Valid {
		rows, err = pg.db.QueryContext(ctx, queryGetAllAfterCursor, cursor.UUID, pageLimit)
	} else {
		rows, err = pg.db.QueryContext(ctx, queryGetAll, pageLimit)
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
	res, err := pg.db.ExecContext(ctx, queryUpdate, p.ID, p.Name, p.Price.MinorAmount, string(p.Price.Currency))
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return entity.ErrNotFound
	}
	return nil
}

func (pg *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := pg.db.ExecContext(ctx, queryDelete, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
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

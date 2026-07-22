package store

import (
	"context"
	"errors"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/alkmc/storefront/internal/event"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Save inserts the product and its created event in one tx.
func (pg *Postgres) Save(
	ctx context.Context, p domain.Product,
) (domain.Product, error) {
	err := pg.withTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(
			ctx, queryInsert, p.ID, p.Name, p.Price.MinorAmount, string(p.Price.Currency), p.Stock,
		).Scan(&p.Version); err != nil {
			return err
		}
		return emitProductEvent(ctx, tx, event.TypeCreated, p)
	})
	if err != nil {
		return domain.Product{}, err
	}
	return p, nil
}

func (pg *Postgres) FindByID(
	ctx context.Context, id uuid.UUID,
) (domain.Product, error) {
	row := pg.pool.QueryRow(ctx, queryGetByID, id)

	var p domain.Product
	if err := row.Scan(
		&p.ID, &p.Name, &p.Price.MinorAmount, &p.Price.Currency, &p.Stock, &p.Version,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Product{}, domain.ErrNotFound
		}
		return domain.Product{}, mapDBError(err)
	}

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
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Price.MinorAmount, &p.Price.Currency, &p.Stock, &p.Version,
		); err != nil {
			return domain.ProductPage{}, mapDBError(err)
		}
		products = append(products, p)
	}

	if err = rows.Err(); err != nil {
		return domain.ProductPage{}, mapDBError(err)
	}

	return productPage(products, limit), nil
}

// Update rewrites the product and stores its updated event in one tx.
func (pg *Postgres) Update(
	ctx context.Context, p domain.Product,
) (domain.Product, error) {
	err := pg.withTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(
			ctx, queryUpdate, p.ID, p.Name, p.Price.MinorAmount, string(p.Price.Currency),
		).Scan(
			&p.ID, &p.Name, &p.Price.MinorAmount, &p.Price.Currency, &p.Stock, &p.Version,
		); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		return emitProductEvent(ctx, tx, event.TypeUpdated, p)
	})
	if err != nil {
		return domain.Product{}, err
	}
	return p, nil
}

// Delete removes the product and stores its deleted event in one tx.
func (pg *Postgres) Delete(ctx context.Context, id uuid.UUID) error {
	return pg.withTx(ctx, func(tx pgx.Tx) error {
		var version int64
		if err := tx.QueryRow(ctx, queryDelete, id).Scan(&version); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			if isProductInUse(err) {
				return domain.ErrProductInUse
			}
			return err
		}
		deleted := domain.Product{ID: id, Version: version}
		return emitProductEvent(ctx, tx, event.TypeDeleted, deleted)
	})
}

// CreateOrder atomically decrements stock, records the order, and stores the purchased event in one tx.
func (pg *Postgres) CreateOrder(
	ctx context.Context, o domain.Order,
) (domain.Order, error) {
	var p domain.Product
	err := pg.withTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, queryPurchase, o.ProductID, o.Quantity).Scan(
			&p.ID, &p.Name, &p.Price.MinorAmount, &p.Price.Currency, &p.Stock, &p.Version,
		); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return purchaseNoRowError(ctx, tx, o.ProductID)
			}
			return err
		}

		// the order snapshots the unit price returned by the decrement
		o.UnitPrice = p.Price
		if err := tx.QueryRow(
			ctx, queryInsertOrder, uuid.UUID(o.ID), uuid.UUID(o.UserID), o.ProductID, o.Quantity,
			o.UnitPrice.MinorAmount, string(o.UnitPrice.Currency),
		).Scan(&o.CreatedAt); err != nil {
			return err
		}

		e, err := event.NewPurchased(p, o.Quantity)
		if err != nil {
			return err
		}
		return insertOutbox(ctx, tx, e)
	})
	if err != nil {
		return domain.Order{}, err
	}
	return o, nil
}

// purchaseNoRowError tells a missing product apart from insufficient stock.
func purchaseNoRowError(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var exists bool
	if err := tx.QueryRow(ctx, queryProductExists, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return domain.ErrNotFound
	}

	return domain.ErrInsufficientStock
}

// isProductInUse reports whether a delete was refused because an order still references the product.
func isProductInUse(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && (pgErr.Code == codeRestrictViolation || pgErr.Code == codeForeignKeyViolation)
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

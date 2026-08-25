package store

import (
	"context"
	"errors"
	"uuid"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/alkmc/storefront/internal/event"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Save inserts the product and its created event in one tx.
func (pg *Postgres) Save(
	ctx context.Context, p domain.Product,
) (domain.Product, error) {
	err := pg.withTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(
			ctx, queryInsert, uuid.UUID(p.ID), p.Name, p.Price.MinorAmount, string(p.Price.Currency), p.Stock,
		).Scan(&p.Version); err != nil {
			return err
		}
		return insertOutbox(ctx, tx, event.New(event.TypeCreated, p))
	})
	if err != nil {
		return domain.Product{}, err
	}
	return p, nil
}

func (pg *Postgres) FindByID(
	ctx context.Context, id domain.ProductID,
) (domain.Product, error) {
	p, err := scanProduct(pg.pool.QueryRow(ctx, queryGetByID, uuid.UUID(id)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Product{}, domain.ErrNotFound
		}
		return domain.Product{}, mapDBError(err)
	}

	return p, nil
}

func (pg *Postgres) FindAll(
	ctx context.Context, cursor domain.Cursor, limit int,
) (domain.ProductPage, error) {
	var (
		rows      pgx.Rows
		err       error
		pageLimit = limit + 1
	)

	if id, ok := cursor.After(); ok {
		rows, err = pg.pool.Query(ctx, queryGetAllAfterCursor, id, pageLimit)
	} else {
		rows, err = pg.pool.Query(ctx, queryGetAll, pageLimit)
	}
	if err != nil {
		return domain.ProductPage{}, mapDBError(err)
	}
	defer rows.Close()

	products := make([]domain.Product, 0, limit+1)
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
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
		updated, err := scanProduct(tx.QueryRow(
			ctx, queryUpdate, uuid.UUID(p.ID), p.Name, p.Price.MinorAmount, string(p.Price.Currency),
		))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		p = updated
		return insertOutbox(ctx, tx, event.New(event.TypeUpdated, p))
	})
	if err != nil {
		return domain.Product{}, err
	}
	return p, nil
}

// Delete removes the product and stores its deleted event in one tx.
func (pg *Postgres) Delete(ctx context.Context, id domain.ProductID) error {
	return pg.withTx(ctx, func(tx pgx.Tx) error {
		var version int64
		if err := tx.QueryRow(ctx, queryDelete, uuid.UUID(id)).Scan(&version); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			if isProductInUse(err) {
				return domain.ErrProductInUse
			}
			return err
		}
		deleted := domain.Product{ID: id, Version: version}
		return insertOutbox(ctx, tx, event.New(event.TypeDeleted, deleted))
	})
}

// CreateOrder atomically decrements stock, records the order, and stores the purchased event in one tx.
// The idempotency key is mandatory (enforced by the transports); the call replays a known key.
func (pg *Postgres) CreateOrder(
	ctx context.Context, o domain.Order, idem domain.IdempotencyKey,
) (domain.Order, bool, error) {
	return pg.createOrderIdempotent(ctx, o, idem)
}

// purchaseTx decrements stock, records the order, and stores the purchased event within the caller's tx.
func purchaseTx(ctx context.Context, tx pgx.Tx, o domain.Order) (domain.Order, error) {
	p, err := scanProduct(tx.QueryRow(ctx, queryPurchase, uuid.UUID(o.ProductID), o.Quantity))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Order{}, purchaseNoRowError(ctx, tx, uuid.UUID(o.ProductID))
		}
		return domain.Order{}, err
	}

	// the order snapshots the unit price returned by the decrement
	o.UnitPrice = p.Price
	if err := tx.QueryRow(
		ctx, queryInsertOrder, uuid.UUID(o.ID), uuid.UUID(o.UserID), uuid.UUID(o.ProductID), o.Quantity,
		o.UnitPrice.MinorAmount, string(o.UnitPrice.Currency),
	).Scan(&o.CreatedAt); err != nil {
		return domain.Order{}, err
	}

	if err := insertOutbox(ctx, tx, event.NewPurchased(p, o.Quantity)); err != nil {
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

// scanProduct reads one products row in the column order of the product queries.
func scanProduct(row pgx.Row) (domain.Product, error) {
	var (
		id uuid.UUID
		p  domain.Product
	)
	if err := row.Scan(
		&id, &p.Name, &p.Price.MinorAmount, &p.Price.Currency, &p.Stock, &p.Version,
	); err != nil {
		return domain.Product{}, err
	}
	p.ID = domain.ProductID(id)
	return p, nil
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

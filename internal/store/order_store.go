package store

import (
	"context"
	"errors"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FindOrder returns the order only when it belongs to userID, anything else is ErrNotFound.
func (pg *Postgres) FindOrder(
	ctx context.Context, userID domain.UserID, orderID domain.OrderID,
) (domain.Order, error) {
	o, err := scanOrder(pg.pool.QueryRow(ctx, queryGetOrderByID, uuid.UUID(orderID), uuid.UUID(userID)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Order{}, domain.ErrNotFound
		}
		return domain.Order{}, mapDBError(err)
	}

	return o, nil
}

// FindOrders returns a keyset page of the user's own orders, newest first.
func (pg *Postgres) FindOrders(
	ctx context.Context, userID domain.UserID, cursor domain.Cursor, limit int,
) (domain.OrderPage, error) {
	var (
		rows      pgx.Rows
		err       error
		pageLimit = limit + 1
	)

	if id, ok := cursor.After(); ok {
		rows, err = pg.pool.Query(ctx, queryGetOrdersAfterCursor, uuid.UUID(userID), id, pageLimit)
	} else {
		rows, err = pg.pool.Query(ctx, queryGetOrders, uuid.UUID(userID), pageLimit)
	}
	if err != nil {
		return domain.OrderPage{}, mapDBError(err)
	}
	defer rows.Close()

	orders := make([]domain.Order, 0, limit+1)
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return domain.OrderPage{}, mapDBError(err)
		}
		orders = append(orders, o)
	}

	if err = rows.Err(); err != nil {
		return domain.OrderPage{}, mapDBError(err)
	}

	return orderPage(orders, limit), nil
}

// scanOrder reads one orders row in the column order of the order queries.
func scanOrder(row pgx.Row) (domain.Order, error) {
	var (
		id     uuid.UUID
		userID uuid.UUID
		pid    uuid.UUID
		o      domain.Order
	)
	if err := row.Scan(
		&id, &userID, &pid, &o.Quantity,
		&o.UnitPrice.MinorAmount, &o.UnitPrice.Currency, &o.CreatedAt,
	); err != nil {
		return domain.Order{}, err
	}
	o.ID = domain.OrderID(id)
	o.UserID = domain.UserID(userID)
	o.ProductID = domain.ProductID(pid)
	return o, nil
}

func orderPage(orders []domain.Order, limit int) domain.OrderPage {
	if len(orders) <= limit {
		return domain.OrderPage{Items: orders}
	}

	return domain.OrderPage{
		Items:   orders[:limit],
		HasMore: true,
	}
}

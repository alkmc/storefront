//go:build integration

package store

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
	"uuid"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/alkmc/storefront/internal/pg/pgtest"
	"github.com/jackc/pgx/v5/pgconn"
)

const testIdempotencyTTL = time.Hour

func setupTestContainerDB(t *testing.T) *Postgres {
	t.Helper()
	return NewPostgres(pgtest.MigratedPool(t), testIdempotencyTTL)
}

func TestMapDBError(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		wantUnavailable bool
	}{
		{
			name:            "nil",
			err:             nil,
			wantUnavailable: false,
		},
		{
			name:            "context canceled",
			err:             context.Canceled,
			wantUnavailable: false,
		},
		{
			name:            "context deadline",
			err:             context.DeadlineExceeded,
			wantUnavailable: false,
		},
		{
			name:            "pg connection failure",
			err:             &pgconn.PgError{Code: "08006"},
			wantUnavailable: true,
		},
		{
			name:            "pg insufficient resources",
			err:             &pgconn.PgError{Code: "53300"},
			wantUnavailable: true,
		},
		{
			name:            "pg admin shutdown",
			err:             &pgconn.PgError{Code: "57P01"},
			wantUnavailable: true,
		},
		{
			name:            "pg unique violation",
			err:             &pgconn.PgError{Code: "23505"},
			wantUnavailable: false,
		},
		{
			name:            "net error",
			err:             &net.OpError{Op: "dial", Err: errors.New("connection refused")},
			wantUnavailable: true,
		},
		{
			name:            "plain error",
			err:             errors.New("boom"),
			wantUnavailable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapDBError(tt.err)
			gotUnavail := errors.Is(got, domain.ErrUnavailable)
			if gotUnavail != tt.wantUnavailable {
				t.Fatalf("mapDBError(%v): got unavailable=%v, want %v", tt.err, gotUnavail, tt.wantUnavailable)
			}
			if tt.err != nil && !errors.Is(got, tt.err) {
				t.Errorf("original error not preserved in chain: %v", got)
			}
		})
	}
}

func testMoney(amount int64) domain.Money {
	return domain.Money{MinorAmount: amount, Currency: domain.CurrencyPLN}
}

func testOrder(userID, productID uuid.UUID, qty int64) domain.Order {
	return domain.Order{
		ID:        domain.OrderID(uuid.NewV7()),
		UserID:    domain.UserID(userID),
		ProductID: domain.ProductID(productID),
		Quantity:  qty,
	}
}

// freshIdem returns a unique idempotency key so independent orders never collide on the key.
func freshIdem() domain.IdempotencyKey {
	return domain.IdempotencyKey(uuid.NewV7().String())
}

// seedProduct saves a product with the given stock and returns its id.
func seedProduct(t *testing.T, repo *Postgres, stock int64) uuid.UUID {
	t.Helper()
	id := uuid.NewV7()
	if _, err := repo.Save(
		t.Context(), domain.Product{ID: domain.ProductID(id), Name: "Widget", Price: testMoney(1000), Stock: stock},
	); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	return id
}

// productStock reloads the product and returns its current stock.
func productStock(t *testing.T, repo *Postgres, id uuid.UUID) int64 {
	t.Helper()
	got, err := repo.FindByID(t.Context(), domain.ProductID(id))
	if err != nil {
		t.Fatalf("reload product: %v", err)
	}
	return got.Stock
}

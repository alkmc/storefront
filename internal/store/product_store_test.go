//go:build integration

package store

import (
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/uuid"
)

func TestPostgres_Save(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	tests := []struct {
		name    string
		product domain.Product
		wantErr bool
	}{
		{
			name: "success",
			product: domain.Product{
				ID: domain.ProductID(uuid.Must(uuid.NewV7())), Name: "Car", Price: testMoney(1050),
			},
			wantErr: false,
		},
		{
			name: "negative price - fails check constraint",
			product: domain.Product{
				ID: domain.ProductID(uuid.Must(uuid.NewV7())), Name: "Bike", Price: testMoney(-500),
			},
			wantErr: true,
		},
		{
			name: "invalid currency - fails check constraint",
			product: domain.Product{
				ID:    domain.ProductID(uuid.Must(uuid.NewV7())),
				Name:  "Bike",
				Price: domain.Money{MinorAmount: 500, Currency: domain.Currency("XXX")},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.Save(ctx, tt.product)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

	t.Run("duplicate id", func(t *testing.T) {
		seededID := uuid.Must(uuid.NewV7())
		if _, err := repo.Save(
			ctx, domain.Product{ID: domain.ProductID(seededID), Name: "Boat", Price: testMoney(1000)},
		); err != nil {
			t.Fatalf("failed to save setup product: %v", err)
		}

		if _, err := repo.Save(
			ctx, domain.Product{ID: domain.ProductID(seededID), Name: "Plane", Price: testMoney(10000)},
		); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestPostgres_FindByID(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: domain.ProductID(id), Name: "Car", Price: testMoney(1050)},
	); err != nil {
		t.Fatalf("failed to save product: %v", err)
	}

	tests := []struct {
		name    string
		id      uuid.UUID
		wantErr bool
	}{
		{
			name:    "existing product",
			id:      id,
			wantErr: false,
		},
		{
			name:    "non-existing product",
			id:      uuid.Must(uuid.NewV7()),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := repo.FindByID(ctx, domain.ProductID(tt.id))
			if tt.wantErr {
				if !errors.Is(err, domain.ErrNotFound) {
					t.Fatalf("expected domain.ErrNotFound, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.ID != domain.ProductID(tt.id) {
				t.Errorf("got %v, want %v", p.ID, tt.id)
			}
		})
	}
}

func TestPostgres_FindAll(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	const (
		pageSize = 1
		allLimit = 50
	)

	page, err := repo.FindAll(ctx, uuid.NullUUID{}, allLimit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 0 || page.HasMore {
		t.Fatalf("expected empty page on empty table, got %+v", page)
	}

	p1 := domain.Product{ID: domain.ProductID(uuid.Must(uuid.NewV7())), Name: "P1", Price: testMoney(100)}
	p2 := domain.Product{ID: domain.ProductID(uuid.Must(uuid.NewV7())), Name: "P2", Price: testMoney(200)}
	for _, p := range []domain.Product{p1, p2} {
		if _, err := repo.Save(ctx, p); err != nil {
			t.Fatalf("failed to save product: %v", err)
		}
	}
	want := []uuid.UUID{uuid.UUID(p1.ID), uuid.UUID(p2.ID)}

	page, err = repo.FindAll(ctx, uuid.NullUUID{}, allLimit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.HasMore {
		t.Error("expected no more when the page holds every product")
	}
	if got := productIDs(page.Items); !slices.Equal(got, want) {
		t.Errorf("full page: got %v, want %v", got, want)
	}

	first, err := repo.FindAll(ctx, uuid.NullUUID{}, pageSize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !first.HasMore {
		t.Error("expected HasMore on a full first page")
	}
	if got := productIDs(first.Items); !slices.Equal(got, want[:pageSize]) {
		t.Errorf("first page: got %v, want %v", got, want[:pageSize])
	}

	cursor := uuid.NullUUID{UUID: uuid.UUID(p1.ID), Valid: true}
	second, err := repo.FindAll(ctx, cursor, pageSize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.HasMore {
		t.Error("expected no more after the last page")
	}
	if got := productIDs(second.Items); !slices.Equal(got, want[pageSize:]) {
		t.Errorf("second page: got %v, want %v", got, want[pageSize:])
	}

	cursor = uuid.NullUUID{UUID: uuid.UUID(p2.ID), Valid: true}
	tail, err := repo.FindAll(ctx, cursor, pageSize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tail.Items) != 0 || tail.HasMore {
		t.Fatalf("expected empty page after the last product, got %+v", tail)
	}
}

func productIDs(products []domain.Product) []uuid.UUID {
	ids := make([]uuid.UUID, len(products))
	for i, p := range products {
		ids[i] = uuid.UUID(p.ID)
	}
	return ids
}

func TestPostgres_Update(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: domain.ProductID(id), Name: "OldName", Price: testMoney(1000), Stock: 5},
	); err != nil {
		t.Fatalf("failed to save product: %v", err)
	}

	tests := []struct {
		name      string
		product   domain.Product
		wantErr   bool
		wantErrIs error
	}{
		{
			name:    "success",
			product: domain.Product{ID: domain.ProductID(id), Name: "NewName", Price: testMoney(2000)},
			wantErr: false,
		},
		{
			name:    "negative price - fails check constraint",
			product: domain.Product{ID: domain.ProductID(id), Name: "NewName", Price: testMoney(-100)},
			wantErr: true,
		},
		{
			name: "non-existing product returns ErrNotFound",
			product: domain.Product{
				ID: domain.ProductID(uuid.Must(uuid.NewV7())), Name: "Ghost", Price: testMoney(100),
			},
			wantErr:   true,
			wantErrIs: domain.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, err := repo.Update(ctx, tt.product)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("expected %v, got %v", tt.wantErrIs, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if updated.Name != tt.product.Name || updated.Price != tt.product.Price {
				t.Errorf("update failed: got %+v", updated)
			}
			if updated.Stock != 5 {
				t.Errorf("stock not preserved by update: got %d, want 5", updated.Stock)
			}
			if updated.Version != 2 {
				t.Errorf("version not bumped: got %d, want 2", updated.Version)
			}
		})
	}
}

func TestPostgres_Delete(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: domain.ProductID(id), Name: "ToDelete", Price: testMoney(1000)},
	); err != nil {
		t.Fatalf("failed to save product: %v", err)
	}

	tests := []struct {
		name    string
		id      uuid.UUID
		wantErr bool
	}{
		{
			name:    "success",
			id:      id,
			wantErr: false,
		},
		{
			name:    "non-existing product returns ErrNotFound",
			id:      uuid.Must(uuid.NewV7()),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Delete(ctx, domain.ProductID(tt.id))
			if tt.wantErr {
				if !errors.Is(err, domain.ErrNotFound) {
					t.Fatalf("expected domain.ErrNotFound, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_, err = repo.FindByID(ctx, domain.ProductID(tt.id))
			if !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("expected domain.ErrNotFound after deletion, got %v", err)
			}
		})
	}
}

func TestPostgres_Delete_ProductInUse(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: domain.ProductID(id), Name: "Widget", Price: testMoney(1000), Stock: 5},
	); err != nil {
		t.Fatalf("failed to seed product: %v", err)
	}
	if _, _, err := repo.CreateOrder(
		ctx, testOrder(uuid.Must(uuid.NewV7()), id, 1), freshIdem(),
	); err != nil {
		t.Fatalf("failed to purchase: %v", err)
	}

	if err := repo.Delete(ctx, domain.ProductID(id)); !errors.Is(err, domain.ErrProductInUse) {
		t.Fatalf("got %v, want domain.ErrProductInUse", err)
	}
	// the product must survive the refused delete
	if _, err := repo.FindByID(ctx, domain.ProductID(id)); err != nil {
		t.Errorf("product missing after refused delete: %v", err)
	}
}

func TestPostgres_CreateOrder(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	tests := []struct {
		name          string
		seedStock     int64
		qty           int64
		wantErrIs     error
		wantRemaining int64
		wantVersion   int64
	}{
		{
			name:          "success decrements stock and bumps version",
			seedStock:     5,
			qty:           2,
			wantRemaining: 3,
			wantVersion:   2,
		},
		{
			name:      "insufficient stock",
			seedStock: 1,
			qty:       2,
			wantErrIs: domain.ErrInsufficientStock,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.Must(uuid.NewV7())
			if _, err := repo.Save(
				ctx, domain.Product{
					ID: domain.ProductID(id), Name: "Widget", Price: testMoney(1000), Stock: tt.seedStock,
				},
			); err != nil {
				t.Fatalf("failed to seed product: %v", err)
			}

			order := testOrder(uuid.Must(uuid.NewV7()), id, tt.qty)
			placed, _, err := repo.CreateOrder(ctx, order, freshIdem())
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("got %v, want %v", err, tt.wantErrIs)
				}
				got, err := repo.FindByID(ctx, domain.ProductID(id))
				if err != nil {
					t.Fatalf("failed to reload product: %v", err)
				}
				if got.Stock != tt.seedStock {
					t.Errorf("stock changed on failed purchase: got %d, want %d", got.Stock, tt.seedStock)
				}
				page, ferr := repo.FindOrders(ctx, order.UserID, uuid.NullUUID{}, 10)
				if ferr != nil || len(page.Items) != 0 {
					t.Errorf("failed purchase left an order: err %v, items %+v", ferr, page.Items)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			reloaded, err := repo.FindByID(ctx, domain.ProductID(id))
			if err != nil {
				t.Fatalf("failed to reload product: %v", err)
			}
			if reloaded.Stock != tt.wantRemaining {
				t.Errorf("got remaining stock %d, want %d", reloaded.Stock, tt.wantRemaining)
			}
			if reloaded.Version != tt.wantVersion {
				t.Errorf("got version %d, want %d", reloaded.Version, tt.wantVersion)
			}
			if placed.ID != order.ID || placed.UserID != order.UserID ||
				placed.ProductID != id || placed.Quantity != tt.qty {
				t.Errorf("order fields not preserved: got %+v", placed)
			}
			if placed.UnitPrice != testMoney(1000) {
				t.Errorf("got unit price %+v, want %+v", placed.UnitPrice, testMoney(1000))
			}
			if placed.CreatedAt.IsZero() {
				t.Error("order created_at not set")
			}
		})
	}

	t.Run("non-existing product", func(t *testing.T) {
		_, _, err := repo.CreateOrder(
			ctx, testOrder(uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), 1), freshIdem(),
		)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("got %v, want domain.ErrNotFound", err)
		}
	})
}

func TestPostgres_CreateOrder_OversellInvariant(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	const (
		initialStock     = 3
		concurrentBuyers = 50
		deniedBuyers     = concurrentBuyers - initialStock
	)

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: domain.ProductID(id), Name: "Widget", Price: testMoney(1000), Stock: initialStock},
	); err != nil {
		t.Fatalf("failed to seed product: %v", err)
	}

	var (
		wg           sync.WaitGroup
		successes    atomic.Int64
		insufficient atomic.Int64
	)
	start := make(chan struct{})
	for range concurrentBuyers {
		wg.Go(func() {
			<-start
			_, _, err := repo.CreateOrder(ctx, testOrder(uuid.Must(uuid.NewV7()), id, 1), freshIdem())
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, domain.ErrInsufficientStock):
				insufficient.Add(1)
			default:
				t.Errorf("unexpected purchase error: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()

	if got := successes.Load(); got != initialStock {
		t.Errorf("got %d successful purchases, want %d", got, initialStock)
	}
	if got := insufficient.Load(); got != deniedBuyers {
		t.Errorf("got %d insufficient-stock errors, want %d", got, deniedBuyers)
	}

	final, err := repo.FindByID(ctx, domain.ProductID(id))
	if err != nil {
		t.Fatalf("failed to reload product: %v", err)
	}
	if final.Stock != 0 {
		t.Errorf("got final stock %d, want 0", final.Stock)
	}
	// version starts at 1 (DEFAULT) and increments once per successful purchase.
	if want := int64(1 + initialStock); final.Version != want {
		t.Errorf("got version %d, want %d", final.Version, want)
	}
}

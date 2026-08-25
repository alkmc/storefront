//go:build integration

package store

import (
	"errors"
	"slices"
	"testing"
	"uuid"

	"github.com/alkmc/storefront/internal/domain"
	"github.com/google/go-cmp/cmp"
)

func TestPostgres_FindOrder(t *testing.T) {
	repo := setupTestContainerDB(t)
	ctx := t.Context()

	productID := uuid.NewV7()
	if _, err := repo.Save(
		ctx, domain.Product{ID: domain.ProductID(productID), Name: "Widget", Price: testMoney(1000), Stock: 5},
	); err != nil {
		t.Fatalf("failed to seed product: %v", err)
	}
	owner := uuid.NewV7()
	stranger := uuid.NewV7()
	placed, _, err := repo.CreateOrder(ctx, testOrder(owner, productID, 2), freshIdem())
	if err != nil {
		t.Fatalf("failed to purchase: %v", err)
	}

	got, err := repo.FindOrder(ctx, domain.UserID(owner), placed.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff := cmp.Diff(placed, got); diff != "" {
		t.Errorf("reloaded order mismatch (-placed +got):\n%s", diff)
	}

	if _, err := repo.FindOrder(ctx, domain.UserID(stranger), placed.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("foreign order: got %v, want domain.ErrNotFound", err)
	}
	if _, err := repo.FindOrder(
		ctx, domain.UserID(owner), domain.OrderID(uuid.NewV7()),
	); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing order: got %v, want domain.ErrNotFound", err)
	}
}

func TestPostgres_FindOrders(t *testing.T) {
	repo := setupTestContainerDB(t)
	ctx := t.Context()

	const (
		aliceOrderCount = 3
		pageSize        = 2
	)

	productID := uuid.NewV7()
	if _, err := repo.Save(
		ctx, domain.Product{ID: domain.ProductID(productID), Name: "Widget", Price: testMoney(1000), Stock: 10},
	); err != nil {
		t.Fatalf("failed to seed product: %v", err)
	}

	alice := domain.UserID(uuid.NewV7())
	bob := domain.UserID(uuid.NewV7())

	placed := make([]domain.OrderID, 0, aliceOrderCount)
	for range aliceOrderCount {
		placed = append(placed, purchaseOrder(t, repo, alice, productID))
	}
	wantNewestFirst := slices.Clone(placed)
	slices.Reverse(wantNewestFirst)

	purchaseOrder(t, repo, bob, productID)

	first, err := repo.FindOrders(ctx, alice, domain.Cursor{}, pageSize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !first.HasMore {
		t.Error("expected HasMore on a full first page")
	}
	if got := orderIDs(first.Items); !slices.Equal(got, wantNewestFirst[:pageSize]) {
		t.Errorf("first page: got %v, want %v", got, wantNewestFirst[:pageSize])
	}

	cursor := domain.NewCursor(uuid.UUID(first.Items[len(first.Items)-1].ID))
	second, err := repo.FindOrders(ctx, alice, cursor, pageSize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.HasMore {
		t.Error("expected no more orders after the last page")
	}
	if got := orderIDs(second.Items); !slices.Equal(got, wantNewestFirst[pageSize:]) {
		t.Errorf("second page: got %v, want %v", got, wantNewestFirst[pageSize:])
	}

	empty, err := repo.FindOrders(ctx, domain.UserID(uuid.NewV7()), domain.Cursor{}, pageSize)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(empty.Items) != 0 || empty.HasMore {
		t.Fatalf("expected empty page, got %+v", empty)
	}
}

func purchaseOrder(t *testing.T, repo *Postgres, userID domain.UserID, productID uuid.UUID) domain.OrderID {
	t.Helper()
	order, _, err := repo.CreateOrder(
		t.Context(), testOrder(uuid.UUID(userID), productID, 1), freshIdem(),
	)
	if err != nil {
		t.Fatalf("failed to purchase: %v", err)
	}
	return order.ID
}

func orderIDs(orders []domain.Order) []domain.OrderID {
	ids := make([]domain.OrderID, len(orders))
	for i, o := range orders {
		ids[i] = o.ID
	}
	return ids
}

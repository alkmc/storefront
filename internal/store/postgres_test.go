//go:build integration

package store

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alkmc/storefront/internal/config"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/alkmc/storefront/migrate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestContainerDB(t *testing.T) (*Postgres, func()) {
	t.Helper()
	ctx := t.Context()

	dbName := "testdb"
	dbUser := "testuser"
	dbPassword := "testpassword"

	pgContainer, err := postgres.Run(
		ctx,
		"postgres:18",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	host, err := pgContainer.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get host: %v", err)
	}

	port, err := pgContainer.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	pgConfig := config.Postgres{
		Host:     host,
		Port:     int(port.Num()),
		User:     dbUser,
		Password: config.Secret(dbPassword),
		Database: dbName,
		SSLMode:  "disable",
	}

	dsn := pgConfig.DSN()

	pgxCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("failed to parse pg config: %v", err)
	}
	migrationDB := stdlib.OpenDB(*pgxCfg)
	if err := migrate.Up(ctx, migrationDB); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}
	if err := migrationDB.Close(); err != nil {
		t.Fatalf("failed to close migration db: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to create pg pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("failed to ping db: %v", err)
	}
	repo := NewPostgres(pool)

	cleanup := func() {
		pool.Close()
		if err := pgContainer.Terminate(context.Background()); err != nil {
			t.Fatalf("failed to terminate pg container: %v", err)
		}
	}

	return repo, cleanup
}

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
			name:    "success",
			product: domain.Product{ID: uuid.Must(uuid.NewV7()), Name: "Car", Price: testMoney(1050)},
			wantErr: false,
		},
		{
			name:    "negative price - fails check constraint",
			product: domain.Product{ID: uuid.Must(uuid.NewV7()), Name: "Bike", Price: testMoney(-500)},
			wantErr: true,
		},
		{
			name: "invalid currency - fails check constraint",
			product: domain.Product{
				ID:    uuid.Must(uuid.NewV7()),
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
			ctx, domain.Product{ID: seededID, Name: "Boat", Price: testMoney(1000)},
		); err != nil {
			t.Fatalf("failed to save setup product: %v", err)
		}

		if _, err := repo.Save(
			ctx, domain.Product{ID: seededID, Name: "Plane", Price: testMoney(10000)},
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
		ctx, domain.Product{ID: id, Name: "Car", Price: testMoney(1050)},
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
			p, err := repo.FindByID(ctx, tt.id)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrNotFound) {
					t.Fatalf("expected domain.ErrNotFound, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.ID != tt.id {
				t.Errorf("got %v, want %v", p.ID, tt.id)
			}
		})
	}
}

func TestPostgres_FindAll(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	page, err := repo.FindAll(ctx, uuid.NullUUID{}, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 0 {
		t.Errorf("expected 0 products, got %d", len(page.Items))
	}
	if page.HasMore {
		t.Error("expected HasMore=false on empty table")
	}

	p1 := domain.Product{ID: uuid.Must(uuid.NewV7()), Name: "P1", Price: testMoney(100)}
	if _, err := repo.Save(ctx, p1); err != nil {
		t.Fatalf("failed to save product 1: %v", err)
	}
	p2 := domain.Product{ID: uuid.Must(uuid.NewV7()), Name: "P2", Price: testMoney(200)}
	if _, err := repo.Save(ctx, p2); err != nil {
		t.Fatalf("failed to save product 2: %v", err)
	}

	page, err = repo.FindAll(ctx, uuid.NullUUID{}, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 2 {
		t.Errorf("expected 2 products, got %d", len(page.Items))
	}
	if page.HasMore {
		t.Error("expected HasMore=false when page is not full")
	}

	// First keyset page: limit 1 yields p1 and signals more.
	first, err := repo.FindAll(ctx, uuid.NullUUID{}, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(first.Items) != 1 || first.Items[0].ID != p1.ID {
		t.Fatalf("expected [p1], got %+v", first.Items)
	}
	if !first.HasMore {
		t.Error("expected HasMore=true on full first page")
	}

	// Second keyset page: cursor at p1 yields p2 and ends the stream.
	cursor := uuid.NullUUID{UUID: first.Items[0].ID, Valid: true}
	second, err := repo.FindAll(ctx, cursor, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != p2.ID {
		t.Fatalf("expected [p2], got %+v", second.Items)
	}
	if second.HasMore {
		t.Error("expected HasMore=false on last page")
	}

	// Cursor at the last product yields an empty final page.
	cursor = uuid.NullUUID{UUID: p2.ID, Valid: true}
	empty, err := repo.FindAll(ctx, cursor, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(empty.Items) != 0 {
		t.Fatalf("expected empty page, got %+v", empty.Items)
	}
	if empty.HasMore {
		t.Error("expected HasMore=false after last product")
	}
}

func TestPostgres_Update(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: id, Name: "OldName", Price: testMoney(1000), Stock: 5},
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
			product: domain.Product{ID: id, Name: "NewName", Price: testMoney(2000)},
			wantErr: false,
		},
		{
			name:    "negative price - fails check constraint",
			product: domain.Product{ID: id, Name: "NewName", Price: testMoney(-100)},
			wantErr: true,
		},
		{
			name:      "non-existing product returns ErrNotFound",
			product:   domain.Product{ID: uuid.Must(uuid.NewV7()), Name: "Ghost", Price: testMoney(100)},
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
		ctx, domain.Product{ID: id, Name: "ToDelete", Price: testMoney(1000)},
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
			err := repo.Delete(ctx, tt.id)
			if tt.wantErr {
				if !errors.Is(err, domain.ErrNotFound) {
					t.Fatalf("expected domain.ErrNotFound, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_, err = repo.FindByID(ctx, tt.id)
			if !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("expected domain.ErrNotFound after deletion, got %v", err)
			}
		})
	}
}

func TestPostgres_Purchase(t *testing.T) {
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
				ctx, domain.Product{ID: id, Name: "Widget", Price: testMoney(1000), Stock: tt.seedStock},
			); err != nil {
				t.Fatalf("failed to seed product: %v", err)
			}

			p, err := repo.Purchase(ctx, id, tt.qty)
			if tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("got %v, want %v", err, tt.wantErrIs)
				}
				got, err := repo.FindByID(ctx, id)
				if err != nil {
					t.Fatalf("failed to reload product: %v", err)
				}
				if got.Stock != tt.seedStock {
					t.Errorf("stock changed on failed purchase: got %d, want %d", got.Stock, tt.seedStock)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Stock != tt.wantRemaining {
				t.Errorf("got remaining stock %d, want %d", p.Stock, tt.wantRemaining)
			}
			if p.Version != tt.wantVersion {
				t.Errorf("got version %d, want %d", p.Version, tt.wantVersion)
			}
		})
	}

	t.Run("non-existing product", func(t *testing.T) {
		_, err := repo.Purchase(ctx, uuid.Must(uuid.NewV7()), 1)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("got %v, want domain.ErrNotFound", err)
		}
	})
}

func TestPostgres_Purchase_OversellInvariant(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	const (
		initialStock     = 3
		concurrentBuyers = 50
	)

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, domain.Product{ID: id, Name: "Widget", Price: testMoney(1000), Stock: initialStock},
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
			_, err := repo.Purchase(ctx, id, 1)
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
	if got := insufficient.Load(); got != concurrentBuyers-initialStock {
		t.Errorf("got %d insufficient-stock errors, want %d", got, concurrentBuyers-initialStock)
	}

	final, err := repo.FindByID(ctx, id)
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

func testMoney(amount int64) domain.Money {
	return domain.Money{MinorAmount: amount, Currency: domain.CurrencyPLN}
}

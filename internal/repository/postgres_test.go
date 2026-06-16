//go:build integration

package repository

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/alkmc/storefront/internal/config"
	"github.com/alkmc/storefront/internal/entity"
	"github.com/alkmc/storefront/internal/migrate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestContainerDB(t *testing.T) (*Repository, func()) {
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
		Host:            host,
		Port:            int(port.Num()),
		User:            dbUser,
		Password:        config.Secret(dbPassword),
		Database:        dbName,
		SSLMode:         "disable",
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: 5 * time.Minute,
	}

	pgxCfg, err := pgx.ParseConfig(pgConfig.DSN())
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

	logger := slog.New(slog.DiscardHandler)
	repo, err := NewPG(ctx, logger, pgConfig)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	cleanup := func() {
		repo.Close()
		if err := pgContainer.Terminate(context.Background()); err != nil {
			t.Fatalf("failed to terminate pg container: %v", err)
		}
	}

	return repo, cleanup
}

func testMoney(amount int64) entity.Money {
	return entity.Money{MinorAmount: amount, Currency: entity.CurrencyPLN}
}

func TestRepository_Save(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	tests := []struct {
		name    string
		product entity.Product
		wantErr bool
	}{
		{
			name:    "success",
			product: entity.Product{ID: uuid.Must(uuid.NewV7()), Name: "Car", Price: testMoney(1050)},
			wantErr: false,
		},
		{
			name:    "negative price - fails check constraint",
			product: entity.Product{ID: uuid.Must(uuid.NewV7()), Name: "Bike", Price: testMoney(-500)},
			wantErr: true,
		},
		{
			name: "invalid currency - fails check constraint",
			product: entity.Product{
				ID:    uuid.Must(uuid.NewV7()),
				Name:  "Bike",
				Price: entity.Money{MinorAmount: 500, Currency: entity.Currency("XXX")},
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
			ctx, entity.Product{ID: seededID, Name: "Boat", Price: testMoney(1000)},
		); err != nil {
			t.Fatalf("failed to save setup product: %v", err)
		}

		if _, err := repo.Save(
			ctx, entity.Product{ID: seededID, Name: "Plane", Price: testMoney(10000)},
		); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRepository_FindByID(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, entity.Product{ID: id, Name: "Car", Price: testMoney(1050)},
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
				if !errors.Is(err, entity.ErrNotFound) {
					t.Fatalf("expected entity.ErrNotFound, got %v", err)
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

func TestRepository_FindAll(t *testing.T) {
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

	p1 := entity.Product{ID: uuid.Must(uuid.NewV7()), Name: "P1", Price: testMoney(100)}
	if _, err := repo.Save(ctx, p1); err != nil {
		t.Fatalf("failed to save product 1: %v", err)
	}
	p2 := entity.Product{ID: uuid.Must(uuid.NewV7()), Name: "P2", Price: testMoney(200)}
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

func TestRepository_Update(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, entity.Product{ID: id, Name: "OldName", Price: testMoney(1000)},
	); err != nil {
		t.Fatalf("failed to save product: %v", err)
	}

	tests := []struct {
		name      string
		product   entity.Product
		wantErr   bool
		wantErrIs error
	}{
		{
			name:    "success",
			product: entity.Product{ID: id, Name: "NewName", Price: testMoney(2000)},
			wantErr: false,
		},
		{
			name:    "negative price - fails check constraint",
			product: entity.Product{ID: id, Name: "NewName", Price: testMoney(-100)},
			wantErr: true,
		},
		{
			name:      "non-existing product returns ErrNotFound",
			product:   entity.Product{ID: uuid.Must(uuid.NewV7()), Name: "Ghost", Price: testMoney(100)},
			wantErr:   true,
			wantErrIs: entity.ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Update(ctx, tt.product)
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
			p, err := repo.FindByID(ctx, tt.product.ID)
			if err != nil {
				t.Fatalf("failed to fetch updated product: %v", err)
			}
			if p.Name != tt.product.Name || p.Price != tt.product.Price {
				t.Errorf("update failed: got %+v", p)
			}
		})
	}
}

func TestRepository_Delete(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(
		ctx, entity.Product{ID: id, Name: "ToDelete", Price: testMoney(1000)},
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
				if !errors.Is(err, entity.ErrNotFound) {
					t.Fatalf("expected entity.ErrNotFound, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_, err = repo.FindByID(ctx, tt.id)
			if !errors.Is(err, entity.ErrNotFound) {
				t.Fatalf("expected entity.ErrNotFound after deletion, got %v", err)
			}
		})
	}
}

package repository

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alkmc/restClean/internal/config"
	"github.com/alkmc/restClean/internal/entity"
	"github.com/alkmc/restClean/internal/migrate"
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

	pgContainer, err := postgres.Run(ctx,
		"postgres:18",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(10*time.Second)),
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
		Password: dbPassword,
		Database: dbName,
		SSLMode:  "disable",
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
	repo, err := NewPG(ctx, logger, pgConfig.DSN())
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	cleanup := func() {
		repo.CloseDB()
		if err := pgContainer.Terminate(context.Background()); err != nil {
			t.Fatalf("failed to terminate pg container: %v", err)
		}
	}

	return repo, cleanup
}

func TestRepository_Save(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	tests := []struct {
		name    string
		product *entity.Product
		wantErr bool
	}{
		{
			name:    "success",
			product: new(entity.Product{ID: uuid.Must(uuid.NewV7()), Name: "Car", Price: 10.5}),
			wantErr: false,
		},
		{
			name:    "negative price - fails check constraint",
			product: new(entity.Product{ID: uuid.Must(uuid.NewV7()), Name: "Bike", Price: -5.0}),
			wantErr: true,
		},
		{
			name:    "duplicate id",
			product: new(entity.Product{ID: uuid.Nil, Name: "Plane", Price: 100.0}),
			wantErr: true,
		},
	}

	preExistingProductID := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(ctx,
		new(entity.Product{ID: preExistingProductID, Name: "Boat", Price: 10.0}),
	); err != nil {
		t.Fatalf("failed to save setup product: %v", err)
	}
	tests[2].product.ID = preExistingProductID

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.Save(ctx, tt.product)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRepository_FindByID(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(ctx, new(entity.Product{ID: id, Name: "Car", Price: 10.5})); err != nil {
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
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if p.ID != tt.id {
					t.Errorf("got %v, want %v", p.ID, tt.id)
				}
			}
		})
	}
}

func TestRepository_FindAll(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	res, err := repo.FindAll(ctx, 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected 0 products, got %d", len(res))
	}

	p1 := new(entity.Product{ID: uuid.Must(uuid.NewV7()), Name: "P1", Price: 1.0})
	if _, err := repo.Save(ctx, p1); err != nil {
		t.Fatalf("failed to save product 1: %v", err)
	}
	p2 := new(entity.Product{ID: uuid.Must(uuid.NewV7()), Name: "P2", Price: 2.0})
	if _, err := repo.Save(ctx, p2); err != nil {
		t.Fatalf("failed to save product 2: %v", err)
	}

	res, err = repo.FindAll(ctx, 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 2 {
		t.Errorf("expected 2 products, got %d", len(res))
	}
}

func TestRepository_Update(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(ctx, new(entity.Product{ID: id, Name: "OldName", Price: 10.0})); err != nil {
		t.Fatalf("failed to save product: %v", err)
	}

	tests := []struct {
		name    string
		product *entity.Product
		wantErr bool
	}{
		{
			name:    "success",
			product: new(entity.Product{ID: id, Name: "NewName", Price: 20.0}),
			wantErr: false,
		},
		{
			name:    "negative price - fails check constraint",
			product: new(entity.Product{ID: id, Name: "NewName", Price: -1.0}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Update(ctx, tt.product)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
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
			}
		})
	}
}

func TestRepository_Delete(t *testing.T) {
	repo, cleanup := setupTestContainerDB(t)
	defer cleanup()
	ctx := t.Context()

	id := uuid.Must(uuid.NewV7())
	if _, err := repo.Save(ctx, new(entity.Product{ID: id, Name: "ToDelete", Price: 10.0})); err != nil {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Delete(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				_, err = repo.FindByID(ctx, tt.id)
				if err == nil {
					t.Fatalf("expected error (Not Found) after deletion")
				}
			}
		})
	}
}

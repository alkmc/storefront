//go:build integration

package store

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/alkmc/storefront/internal/config"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/alkmc/storefront/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const testIdempotencyTTL = time.Hour

func setupTestContainerDB(t *testing.T) (*Postgres, func()) {
	t.Helper()
	ctx := t.Context()

	dbName := "testdb"
	dbUser := "testuser"
	dbPassword := "testpassword"

	// the postgres image logs the ready line twice: first the init bootstrap, then the real start
	const dbReadyLogOccurrences = 2

	pgContainer, err := postgres.Run(
		ctx,
		"postgres:18",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(dbReadyLogOccurrences).
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
	if err := migrations.Up(ctx, migrationDB); err != nil {
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
	repo := NewPostgres(pool, testIdempotencyTTL)

	cleanup := func() {
		pool.Close()
		if err := pgContainer.Terminate(context.Background()); err != nil {
			t.Fatalf("failed to terminate pg container: %v", err)
		}
	}

	return repo, cleanup
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
		ID:        domain.OrderID(uuid.Must(uuid.NewV7())),
		UserID:    domain.UserID(userID),
		ProductID: domain.ProductID(productID),
		Quantity:  qty,
	}
}

// freshIdem returns a unique idempotency key so independent orders never collide on the key.
func freshIdem() domain.IdempotencyKey {
	return domain.IdempotencyKey(uuid.Must(uuid.NewV7()).String())
}

// seedProduct saves a product with the given stock and returns its id.
func seedProduct(t *testing.T, repo *Postgres, stock int64) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
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

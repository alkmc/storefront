//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/alkmc/storefront/internal/config"
	"github.com/alkmc/storefront/internal/domain"
	"github.com/alkmc/storefront/migrations"
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
	repo := NewPostgres(pool)

	cleanup := func() {
		pool.Close()
		if err := pgContainer.Terminate(context.Background()); err != nil {
			t.Fatalf("failed to terminate pg container: %v", err)
		}
	}

	return repo, cleanup
}

func testMoney(amount int64) domain.Money {
	return domain.Money{MinorAmount: amount, Currency: domain.CurrencyPLN}
}

func testOrder(userID, productID uuid.UUID, qty int64) domain.Order {
	return domain.Order{
		ID:        domain.OrderID(uuid.Must(uuid.NewV7())),
		UserID:    domain.UserID(userID),
		ProductID: productID,
		Quantity:  qty,
	}
}

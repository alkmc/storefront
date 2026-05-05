package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	"github.com/alkmc/restClean/pkg/entity"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type pgRepository struct {
	logger *slog.Logger
	db     *sql.DB
}

// NewPG creates a new PostgreSQL repository
func NewPG(l *slog.Logger) (Repository, error) {
	dbConn, err := getDbConn()
	if err != nil {
		return nil, err
	}
	pdb, err := sql.Open("pgx", dbConn)
	if err != nil {
		return nil, fmt.Errorf("failed to open pgx connection: %w", err)
	}

	if err := pdb.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping pg database: %w", err)
	}
	l.Info("successfully connected to db")

	if _, err := pdb.Exec(sqlSchema); err != nil {
		l.Error("failed to execute sql schema", slog.Any("error", err), slog.String("schema", sqlSchema))
	}
	return &pgRepository{logger: l, db: pdb}, nil
}

func (pg *pgRepository) CloseDB() {
	if err := pg.db.Close(); err != nil {
		pg.logger.Error("failed to close db connection", slog.Any("error", err))
	}
	pg.logger.Info("connection to db closed")
}

func (pg *pgRepository) Save(ctx context.Context, p *entity.Product) (*entity.Product, error) {
	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	stmt, err := tx.PrepareContext(ctx, queryInsert)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	if _, err := stmt.ExecContext(ctx, p.ID, p.Name, p.Price); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p, nil
}

func (pg *pgRepository) FindByID(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
	row := pg.db.QueryRowContext(ctx, queryGetByID, id)

	var p entity.Product
	if err := row.Scan(&p.ID, &p.Name, &p.Price); err != nil {
		return nil, err
	}
	return &p, nil
}

func (pg *pgRepository) FindAll(ctx context.Context) ([]entity.Product, error) {
	rows, err := pg.db.QueryContext(ctx, queryGetAll)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []entity.Product
	for rows.Next() {
		var p entity.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Price); err != nil {
			return nil, err
		}
		products = append(products, p)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return products, nil
}

func (pg *pgRepository) Update(ctx context.Context, p *entity.Product) error {
	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, queryUpdate)
	if err != nil {
		return err
	}
	defer stmt.Close()

	if _, err := stmt.ExecContext(ctx, p.ID, p.Name, p.Price); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (pg *pgRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := pg.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, queryDelete)
	if err != nil {
		return err
	}
	defer stmt.Close()

	if _, err = stmt.ExecContext(ctx, id); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func getEnvVars() (map[string]string, error) {
	const req = "environment variable %q is required"
	keys := []string{
		"PG_HOST",
		"PG_PORT",
		"PG_USER",
		"PG_PASSWORD",
		"PG_DB",
	}
	t := map[string]string{}
	for _, key := range keys {
		v := os.Getenv(key)
		if v == "" {
			return nil, fmt.Errorf(req, key)
		}
		t[key] = v
	}
	return t, nil
}

func getDbConn() (string, error) {
	const connStr = "host=%s port=%s user=%s password=%s dbname=%s sslmode=disable"

	e, err := getEnvVars()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(connStr,
		e["PG_HOST"],
		e["PG_PORT"],
		e["PG_USER"],
		e["PG_PASSWORD"],
		e["PG_DB"]), nil
}

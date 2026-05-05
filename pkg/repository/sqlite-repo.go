package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	"github.com/alkmc/restClean/pkg/entity"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type sqliteRepo struct {
	logger *slog.Logger
	db     *sql.DB
}

// NewSQLite creates a new SQL lite repository
func NewSQLite(l *slog.Logger) (Repository, error) {
	if err := os.Remove("./prods.db"); err != nil && !os.IsNotExist(err) {
		l.Error("failed to remove existing db", slog.Any("error", err))
	}

	sdb, err := sql.Open("sqlite3", "./prods.db")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite connection: %w", err)
	}
	if err := sdb.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	if _, err = sdb.Exec(sqlSchema); err != nil {
		l.Error("failed to execute sql schema", slog.Any("error", err), slog.String("schema", sqlSchema))
	}
	return &sqliteRepo{logger: l, db: sdb}, nil
}

func (s *sqliteRepo) CloseDB() {
	if err := s.db.Close(); err != nil {
		s.logger.Error("failed to close database", slog.Any("error", err))
	}
	s.logger.Info("connection to db closed")
}

func (s *sqliteRepo) Save(ctx context.Context, p *entity.Product) (*entity.Product, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	stmt, err := tx.PrepareContext(ctx, queryInsert)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	if _, err = stmt.ExecContext(ctx, p.ID, p.Name, p.Price); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *sqliteRepo) FindByID(ctx context.Context, id uuid.UUID) (*entity.Product, error) {
	row := s.db.QueryRowContext(ctx, queryGetByID, id)

	var p entity.Product
	if err := row.Scan(&p.ID, &p.Name, &p.Price); err != nil {
		return nil, err
	}

	return &p, nil
}

func (s *sqliteRepo) FindAll(ctx context.Context) ([]entity.Product, error) {
	rows, err := s.db.QueryContext(ctx, queryGetAll)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []entity.Product
	for rows.Next() {
		var p entity.Product
		if err = rows.Scan(&p.ID, &p.Name, &p.Price); err != nil {
			return nil, err
		}
		products = append(products, p)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}
	return products, nil
}

func (s *sqliteRepo) Update(ctx context.Context, p *entity.Product) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, queryUpdate)
	if err != nil {
		return err
	}
	defer stmt.Close()

	if _, err := stmt.ExecContext(ctx, p.Name, p.Price, p.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *sqliteRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
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

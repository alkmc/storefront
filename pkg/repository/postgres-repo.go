package repository

import (
	"fmt"
	"log"
	"os"

	"github.com/alkmc/restClean/pkg/entity"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type pgRepository struct {
	db *sqlx.DB
}

// NewPG creates a new PostgreSQL repository
func NewPG() Repository {
	dbConn := getDbConn()
	pdb, err := sqlx.Open("postgres", dbConn)
	if err != nil {
		log.Fatal(err)
	}

	if err := pdb.Ping(); err != nil {
		log.Fatal(err)
	}
	log.Println("successfully connected to db")

	if _, err := pdb.Exec(sqlSchema); err != nil {
		log.Printf("%q: %s", err, sqlSchema)
	}
	return &pgRepository{db: pdb}
}

func (pg *pgRepository) CloseDB() {
	if err := pg.db.Close(); err != nil {
		log.Println("failed to close db connection")
	}
	log.Println("connection to db closed")
}

func (pg *pgRepository) Save(p *entity.Product) (*entity.Product, error) {
	tx, err := pg.db.Begin()
	if err != nil {
		return nil, err
	}

	stmt, err := tx.Prepare(queryInsert)
	if err != nil {
		return nil, err
	}

	defer stmt.Close()
	if _, err := stmt.Exec(p.ID, p.Name, p.Price); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return p, nil
}

func (pg *pgRepository) FindByID(id uuid.UUID) (*entity.Product, error) {
	row := pg.db.QueryRow(queryGetByID, id)

	var p entity.Product
	if err := row.Scan(&p.ID, &p.Name, &p.Price); err != nil {
		return nil, err
	}
	return &p, nil
}

func (pg *pgRepository) FindAll() ([]entity.Product, error) {
	rows, err := pg.db.Query(queryGetAll)
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

func (pg *pgRepository) Update(p *entity.Product) error {
	tx, err := pg.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(queryUpdate)
	if err != nil {
		return err
	}
	defer stmt.Close()

	if _, err := stmt.Exec(p.ID, p.Name, p.Price); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (pg *pgRepository) Delete(id uuid.UUID) error {
	tx, err := pg.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(queryDelete)
	if err != nil {
		return err
	}
	defer stmt.Close()

	if _, err = stmt.Exec(id); err != nil {
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

func getDbConn() string {
	const connStr = "host=%s port=%s user=%s password=%s dbname=%s sslmode=disable"

	e, err := getEnvVars()
	if err != nil {
		log.Fatal(err)
	}

	return fmt.Sprintf(connStr,
		e["PG_HOST"],
		e["PG_PORT"],
		e["PG_USER"],
		e["PG_PASSWORD"],
		e["PG_DB"])
}

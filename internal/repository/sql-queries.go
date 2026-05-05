package repository

const (
	queryInsert  = "INSERT INTO products (uid, name, price) VALUES ($1, $2, $3);"
	queryGetByID = "SELECT * FROM products WHERE uid = $1;"
	queryGetAll  = "SELECT * FROM products;"
	queryUpdate  = "UPDATE products SET name = $2, price = $3 WHERE uid = $1;"
	queryDelete  = "DELETE FROM products WHERE uid = $1;"
)

const sqlSchema = `CREATE TABLE IF NOT EXISTS products (
	uid UUID PRIMARY KEY,
	name VARCHAR(100) NOT NULL,
	price NUMERIC(10,2) NOT NULL CHECK (price > 0)
);`

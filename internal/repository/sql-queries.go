package repository

const (
	queryInsert  = "INSERT INTO products (id, name, price) VALUES ($1, $2, $3);"
	queryGetByID = "SELECT id, name, price FROM products WHERE id = $1;"
	queryGetAll  = "SELECT id, name, price FROM products ORDER BY id LIMIT $1 OFFSET $2;"
	queryUpdate  = "UPDATE products SET name = $2, price = $3 WHERE id = $1;"
	queryDelete  = "DELETE FROM products WHERE id = $1;"
)

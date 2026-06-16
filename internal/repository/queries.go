package repository

const (
	queryInsert = `
		INSERT INTO products (id, name, price_minor, currency)
		VALUES ($1, $2, $3, $4);`
	queryGetByID = `
		SELECT id, name, price_minor, currency
		FROM products
		WHERE id = $1;`
	queryGetAll = `
		SELECT id, name, price_minor, currency
		FROM products
		ORDER BY id
		LIMIT $1;`
	queryGetAllAfterCursor = `
		SELECT id, name, price_minor, currency
		FROM products
		WHERE id > $1
		ORDER BY id
		LIMIT $2;`
	queryUpdate = `
		UPDATE products
		SET name = $2, price_minor = $3, currency = $4
		WHERE id = $1;`
	queryDelete = `
		DELETE FROM products
		WHERE id = $1;`
)

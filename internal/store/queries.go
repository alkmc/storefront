package store

const (
	queryInsert = `
		INSERT INTO products (id, name, price_minor, currency, stock)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING version;`
	queryGetByID = `
		SELECT id, name, price_minor, currency, stock, version
		FROM products
		WHERE id = $1;`
	queryGetAll = `
		SELECT id, name, price_minor, currency, stock, version
		FROM products
		ORDER BY id
		LIMIT $1;`
	queryGetAllAfterCursor = `
		SELECT id, name, price_minor, currency, stock, version
		FROM products
		WHERE id > $1
		ORDER BY id
		LIMIT $2;`
	queryUpdate = `
		UPDATE products
		SET name = $2, price_minor = $3, currency = $4, version = version + 1
		WHERE id = $1
		RETURNING id, name, price_minor, currency, stock, version;`
	queryDelete = `
		DELETE FROM products
		WHERE id = $1;`
	queryPurchase = `
		UPDATE products
		SET stock = stock - $2, version = version + 1
		WHERE id = $1 AND stock >= $2
		RETURNING id, name, price_minor, currency, stock, version;`
	queryProductExists = `
		SELECT EXISTS(SELECT 1 FROM products WHERE id = $1);`
)

package store

// OutboxChannel is the NOTIFY channel that wakes the outbox relay, LISTEN and NOTIFY share it.
const OutboxChannel = "outbox_wakeup"

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
		WHERE id = $1
		RETURNING version + 1;`
	queryPurchase = `
		UPDATE products
		SET stock = stock - $2, version = version + 1
		WHERE id = $1 AND stock >= $2
		RETURNING id, name, price_minor, currency, stock, version;`
	queryProductExists = `
		SELECT EXISTS(SELECT 1 FROM products WHERE id = $1);`
	queryInsertOutbox = `
		INSERT INTO outbox (message_id, event_type, payload)
		VALUES ($1, $2, $3);`
	queryNotifyOutbox = `
		SELECT pg_notify('` + OutboxChannel + `', '');`
	queryClaimOutbox = `
		SELECT id, message_id, event_type, payload, attempts, created_at
		FROM outbox
		WHERE next_attempt_at <= now()
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED;`
	queryDeleteOutbox = `
		DELETE FROM outbox
		WHERE id = ANY($1);`
	queryBumpOutbox = `
		UPDATE outbox
		SET attempts = attempts + 1,
		    next_attempt_at = now() + make_interval(secs => LEAST(POWER(2, attempts), 30))
		WHERE id = ANY($1);`
	queryDeadOutbox = `
		INSERT INTO outbox_dead (id, message_id, event_type, payload, attempts, created_at, last_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7);`
	queryInsertOrder = `
		INSERT INTO orders (id, user_id, product_id, quantity, unit_price_minor, currency)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at;`
	queryGetOrderByID = `
		SELECT id, user_id, product_id, quantity, unit_price_minor, currency, created_at
		FROM orders
		WHERE id = $1 AND user_id = $2;`
	queryGetOrders = `
		SELECT id, user_id, product_id, quantity, unit_price_minor, currency, created_at
		FROM orders
		WHERE user_id = $1
		ORDER BY id DESC
		LIMIT $2;`
	queryGetOrdersAfterCursor = `
		SELECT id, user_id, product_id, quantity, unit_price_minor, currency, created_at
		FROM orders
		WHERE user_id = $1 AND id < $2
		ORDER BY id DESC
		LIMIT $3;`
	queryInsertIdempotency = `
		INSERT INTO idempotency_keys (user_id, key, request_hash, expires_at)
		VALUES ($1, $2, $3, now() + make_interval(secs => $4))
		ON CONFLICT DO NOTHING;`
	querySelectIdempotency = `
		SELECT ik.request_hash, o.id, o.product_id, o.quantity, o.unit_price_minor, o.currency, o.created_at
		FROM idempotency_keys ik
		JOIN orders o ON o.id = ik.order_id
		WHERE ik.user_id = $1 AND ik.key = $2;`
	queryUpdateIdempotencyResult = `
		UPDATE idempotency_keys
		SET order_id = $3
		WHERE user_id = $1 AND key = $2;`
	queryPurgeIdempotency = `
		DELETE FROM idempotency_keys
		WHERE expires_at < now();`
)

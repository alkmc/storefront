-- +goose Up
CREATE TABLE products (
    id UUID PRIMARY KEY,
    price_minor BIGINT NOT NULL CHECK (price_minor > 0),
    version BIGINT NOT NULL DEFAULT 1,
    stock INTEGER NOT NULL DEFAULT 0 CHECK (stock >= 0),
    name VARCHAR(100) NOT NULL,
    -- Keep this list in sync with internal/domain/money.go.
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('PLN', 'EUR', 'USD', 'GBP', 'CHF'))
);

-- +goose Down
DROP TABLE IF EXISTS products;

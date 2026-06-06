-- +goose Up
CREATE TABLE products (
    id UUID PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    price_minor BIGINT NOT NULL CHECK (price_minor > 0),
    -- Keep this list in sync with internal/entity/money.go.
    currency VARCHAR(3) NOT NULL CHECK (currency IN ('PLN', 'EUR', 'USD', 'GBP', 'CHF'))
);

-- +goose Down
DROP TABLE IF EXISTS products;

-- +goose Up
CREATE TABLE products (
    id UUID PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    price NUMERIC(10,2) NOT NULL CHECK (price > 0)
);

-- +goose Down
DROP TABLE IF EXISTS products;

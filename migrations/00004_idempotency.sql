-- +goose Up
CREATE TABLE idempotency_keys (
    user_id UUID NOT NULL,
    key TEXT NOT NULL,
    request_hash BYTEA NOT NULL,
    order_id UUID REFERENCES orders (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, key)
);
CREATE INDEX idempotency_keys_expires_at_idx ON idempotency_keys (expires_at);

-- +goose Down
DROP TABLE IF EXISTS idempotency_keys;

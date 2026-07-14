-- +goose Up
CREATE TABLE outbox (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    message_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox_dead (
    id BIGINT PRIMARY KEY,
    message_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    attempts INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    dead_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS outbox_dead;
DROP TABLE IF EXISTS outbox;

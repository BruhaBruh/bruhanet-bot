-- +goose Up
CREATE TABLE outbox (
    id         bigserial PRIMARY KEY,
    event_type text NOT NULL,
    payload    jsonb NOT NULL
);

-- +goose Down
DROP TABLE outbox;

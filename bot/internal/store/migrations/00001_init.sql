-- +goose Up
CREATE TABLE users (
    telegram_id bigint PRIMARY KEY,
    username    text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE payments (
    id               bigserial PRIMARY KEY,
    telegram_id      bigint NOT NULL REFERENCES users,
    username         text NOT NULL,
    months           int NOT NULL,
    price            int NOT NULL,
    status           text NOT NULL CHECK (status IN ('pending', 'confirmed', 'declined', 'cancelled')),
    prev_expire_at   timestamptz NOT NULL,
    advance_granted  bool NOT NULL DEFAULT false,
    admin_message_id int,
    created_at       timestamptz NOT NULL DEFAULT now(),
    submitted_at     timestamptz,
    resolved_at      timestamptz
);

CREATE UNIQUE INDEX payments_one_pending ON payments (telegram_id) WHERE status = 'pending';
CREATE INDEX payments_telegram_id_id ON payments (telegram_id, id);

CREATE TABLE backups (
    id         bigserial PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    size_bytes bigint NOT NULL
);

-- +goose Down
DROP TABLE backups, payments, users;

-- +goose Up
CREATE TABLE broadcasts (
    id              uuid PRIMARY KEY,
    idempotency_key uuid UNIQUE NOT NULL,
    text            text NOT NULL,
    photo           text,
    total           int NOT NULL,
    sent            int NOT NULL DEFAULT 0,
    failed          int NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    finished_at     timestamptz
);

CREATE TABLE direct_messages (
    id          bigserial PRIMARY KEY,
    telegram_id bigint NOT NULL,
    text        text NOT NULL,
    photo       text,
    status      text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX direct_messages_telegram_id_idx ON direct_messages (telegram_id, id);

-- +goose Down
DROP TABLE direct_messages;
DROP TABLE broadcasts;

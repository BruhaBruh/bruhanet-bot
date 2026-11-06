-- +goose Up
-- Дни, выданные админом из панели.
CREATE TABLE grants (
    id           bigserial PRIMARY KEY,
    telegram_id  bigint NOT NULL,
    days         int NOT NULL CHECK (days BETWEEN 1 AND 365),
    created_at   timestamptz NOT NULL DEFAULT now(),
    by_admin_id  bigint NOT NULL
);
CREATE INDEX grants_user_idx ON grants (telegram_id, id DESC);

-- +goose Down
DROP TABLE grants;

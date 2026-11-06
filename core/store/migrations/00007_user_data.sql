-- +goose Up
ALTER TABLE users
    ADD COLUMN first_name          text,
    ADD COLUMN last_name           text,
    ADD COLUMN last_seen_at        timestamptz,
    ADD COLUMN expire_at           timestamptz,
    ADD COLUMN panel_status        text,
    ADD COLUMN synced_at           timestamptz,
    ADD COLUMN blocked_bot_at      timestamptz,
    ADD COLUMN broadcasts_received int NOT NULL DEFAULT 0,
    ADD COLUMN blocked_at          timestamptz;

-- Итог последней успешной синхронизации с панелью: одна строка.
CREATE TABLE panel_sync (
    id          bool PRIMARY KEY DEFAULT true CHECK (id),
    finished_at timestamptz NOT NULL,
    total       int NOT NULL,
    updated     int NOT NULL
);

-- +goose Down
DROP TABLE panel_sync;
ALTER TABLE users
    DROP COLUMN blocked_at,
    DROP COLUMN broadcasts_received,
    DROP COLUMN blocked_bot_at,
    DROP COLUMN synced_at,
    DROP COLUMN panel_status,
    DROP COLUMN expire_at,
    DROP COLUMN last_seen_at,
    DROP COLUMN last_name,
    DROP COLUMN first_name;

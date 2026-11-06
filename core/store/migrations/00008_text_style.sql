-- +goose Up
-- Стиль кнопки живёт в той же строке; body NULL — подпись не менялась, изменён только стиль.
ALTER TABLE texts
    ADD COLUMN style text,
    ALTER COLUMN body DROP NOT NULL;

-- +goose Down
DELETE FROM texts WHERE body IS NULL;
ALTER TABLE texts
    ALTER COLUMN body SET NOT NULL,
    DROP COLUMN style;

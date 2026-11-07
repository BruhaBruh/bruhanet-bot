-- +goose Up
-- Идемпотентность выдачи подарков из панели: повтор POST /api/gifts с тем же ключом (потерянный ответ, двойной клик)
-- возвращает ту же пачку, а не создаёт вторую.
ALTER TABLE gifts ADD COLUMN batch_key uuid;
CREATE INDEX gifts_batch_key_idx ON gifts (batch_key) WHERE batch_key IS NOT NULL;

-- +goose Down
DROP INDEX gifts_batch_key_idx;
ALTER TABLE gifts DROP COLUMN batch_key;

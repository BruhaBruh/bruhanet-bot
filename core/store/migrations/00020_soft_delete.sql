-- +goose Up
-- Удаление в админке. Промокод с историей (активации, платежи) и купленный подарок не стираются физически:
-- deleted_at скрывает их из списков, код остаётся занятым (UNIQUE), платежи и активации сохраняют смысл.
ALTER TABLE promo_codes ADD COLUMN deleted_at timestamptz;
ALTER TABLE gifts ADD COLUMN deleted_at timestamptz;

-- +goose Down
ALTER TABLE gifts DROP COLUMN deleted_at;
ALTER TABLE promo_codes DROP COLUMN deleted_at;

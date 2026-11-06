-- +goose Up
-- file_id чека в Telegram: панель показывает картинку через getFile. Старые платежи — NULL, чек остался в боте.
ALTER TABLE payments ADD COLUMN photo_file_id text;

-- +goose Down
ALTER TABLE payments DROP COLUMN photo_file_id;

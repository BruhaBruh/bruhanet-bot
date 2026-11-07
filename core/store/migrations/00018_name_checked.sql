-- +goose Up
-- Когда бот последний раз спрашивал у Telegram (getChat) имя юзера; NULL — ещё не спрашивал.
ALTER TABLE users ADD COLUMN name_checked_at timestamptz;
-- +goose Down
ALTER TABLE users DROP COLUMN name_checked_at;

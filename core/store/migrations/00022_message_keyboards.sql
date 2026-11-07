-- +goose Up
-- Клавиатура сообщения (конструктор): ряды кнопок {id, подпись, стиль, действие, условия}. Нет строки — клавиатура
-- по умолчанию из кода. Старая таблица text_keyboards (общие раскладки) остаётся: её переносит
-- Store.MigrateLegacyKeyboards при `bot migrate`.
CREATE TABLE message_keyboards (
    text_key   text PRIMARY KEY,
    rows       jsonb       NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE message_keyboards;

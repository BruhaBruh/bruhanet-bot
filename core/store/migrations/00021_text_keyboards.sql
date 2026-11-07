-- +goose Up
-- Раскладка клавиатуры, правленная в админке: ряды видимых кнопок и скрытые. Нет строки — раскладка из кода.
CREATE TABLE text_keyboards (
    keyboard_id text PRIMARY KEY,
    rows        jsonb       NOT NULL,
    hidden      jsonb       NOT NULL DEFAULT '[]',
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE text_keyboards;

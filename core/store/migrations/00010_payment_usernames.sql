-- +goose Up
-- Бот записывал telegram id вместо отсутствующего username. Настоящий username начинается с буквы, цифрами он быть не может.
UPDATE users SET username = '' WHERE username = telegram_id::text;
UPDATE payments SET username = '' WHERE username = telegram_id::text;

-- +goose Down
-- Обратно не восстанавливаем: пустой username и так значит «нет username».
SELECT 1;

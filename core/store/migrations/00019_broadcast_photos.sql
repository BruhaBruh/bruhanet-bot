-- +goose Up
-- Картинки рассылок и личных сообщений: байты лежат в БД, в Telegram уходят только при доставке; file_id воркер
-- запоминает после первой отправки, остальным получателям фото идёт по нему.
CREATE TABLE broadcast_photos (
    id           uuid PRIMARY KEY,
    content      bytea       NOT NULL,
    content_type text        NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png', 'image/webp')),
    size         int         NOT NULL,
    file_id      text,
    created_at   timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE broadcasts ADD COLUMN photo_id uuid REFERENCES broadcast_photos (id) ON DELETE SET NULL;
ALTER TABLE direct_messages ADD COLUMN photo_id uuid REFERENCES broadcast_photos (id) ON DELETE SET NULL;
CREATE INDEX broadcasts_photo_id_idx ON broadcasts (photo_id) WHERE photo_id IS NOT NULL;
CREATE INDEX direct_messages_photo_id_idx ON direct_messages (photo_id) WHERE photo_id IS NOT NULL;
-- +goose Down
ALTER TABLE direct_messages DROP COLUMN photo_id;
ALTER TABLE broadcasts DROP COLUMN photo_id;
DROP TABLE broadcast_photos;

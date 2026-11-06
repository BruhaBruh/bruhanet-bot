-- +goose Up
ALTER TABLE promo_codes
    ADD COLUMN starts_at timestamptz,
    ADD CONSTRAINT promo_codes_starts_check CHECK (starts_at IS NULL OR expires_at IS NULL OR starts_at < expires_at);

-- +goose Down
ALTER TABLE promo_codes DROP CONSTRAINT promo_codes_starts_check, DROP COLUMN starts_at;

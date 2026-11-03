-- +goose Up
CREATE TABLE promo_codes (
    id              bigserial PRIMARY KEY,
    code            text UNIQUE NOT NULL,
    kind            text NOT NULL CHECK (kind IN ('percent', 'rub', 'days')),
    value           int NOT NULL,
    max_activations int,
    expires_at      timestamptz,
    only_new        bool NOT NULL DEFAULT false,
    telegram_id     bigint,
    disabled        bool NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE promo_activations (
    id          bigserial PRIMARY KEY,
    promo_id    bigint NOT NULL REFERENCES promo_codes,
    telegram_id bigint NOT NULL,
    payment_id  bigint REFERENCES payments,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (promo_id, telegram_id)
);

ALTER TABLE payments
    ADD COLUMN promo_id bigint REFERENCES promo_codes,
    ADD COLUMN base_price int;

-- +goose Down
ALTER TABLE payments DROP COLUMN base_price, DROP COLUMN promo_id;
DROP TABLE promo_activations;
DROP TABLE promo_codes;

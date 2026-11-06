-- +goose Up
-- Подарки: платёж за подарок (payments.kind = 'gift') не меняет срок и устройства покупателя, а открывает ссылку-код.
ALTER TABLE payments DROP CONSTRAINT payments_kind_check;
ALTER TABLE payments ADD CONSTRAINT payments_kind_check CHECK (kind IN ('tariff', 'devices', 'gift'));

-- status: pending — платёж за подарок ещё не подтверждён (код не показываем), ready — ссылкой можно воспользоваться
-- (подарок админа создаётся сразу ready), redeemed — активирован, cancelled — платёж отклонён или подарок отменён.
-- code: случайные 128 бит в base64url, по нему же ссылка t.me/<бот>?start=gift_<code>.
CREATE TABLE gifts (
    id                bigserial PRIMARY KEY,
    code              text NOT NULL UNIQUE,
    months            int NOT NULL CHECK (months BETWEEN 1 AND 120),
    status            text NOT NULL CHECK (status IN ('pending', 'ready', 'redeemed', 'cancelled')),
    buyer_id          bigint,
    payment_id        bigint UNIQUE REFERENCES payments,
    created_by_admin  boolean NOT NULL DEFAULT false,
    redeemed_by       bigint,
    redeemed_at       timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX gifts_status_idx ON gifts (status, id DESC);

-- Подарок на 120 месяцев — 3600 дней: выдача пишется в grants (source gift).
ALTER TABLE grants DROP CONSTRAINT grants_days_check;
ALTER TABLE grants ADD CONSTRAINT grants_days_check CHECK (days BETWEEN 1 AND 3650);

-- +goose Down
DELETE FROM grants WHERE days > 365;
ALTER TABLE grants DROP CONSTRAINT grants_days_check;
ALTER TABLE grants ADD CONSTRAINT grants_days_check CHECK (days BETWEEN 1 AND 365);
DROP TABLE gifts;
DELETE FROM payments WHERE kind = 'gift';
ALTER TABLE payments DROP CONSTRAINT payments_kind_check;
ALTER TABLE payments ADD CONSTRAINT payments_kind_check CHECK (kind IN ('tariff', 'devices'));

-- +goose Up
-- Купленные сверх базы устройства юзера. Лимит в панели = база + extra_devices, пишется только при событии с устройствами.
ALTER TABLE users ADD COLUMN extra_devices int NOT NULL DEFAULT 0 CHECK (extra_devices >= 0);

-- kind: tariff — подписка, devices — докупка устройств (срок не меняет).
-- extra_devices: для devices — сколько куплено, для tariff — сколько устройств сверх базы у юзера после платежа.
ALTER TABLE payments
    ADD COLUMN kind          text NOT NULL DEFAULT 'tariff' CHECK (kind IN ('tariff', 'devices')),
    ADD COLUMN extra_devices int  NOT NULL DEFAULT 0 CHECK (extra_devices >= 0);

-- +goose Down
ALTER TABLE payments DROP COLUMN extra_devices, DROP COLUMN kind;
ALTER TABLE users DROP COLUMN extra_devices;

-- +goose Up
-- Кто пригласил: пишется один раз, при первом создании строки юзера по ссылке /start ref_<id>; не перезаписывается.
ALTER TABLE users ADD COLUMN referred_by bigint CHECK (referred_by <> telegram_id);
CREATE INDEX users_referred_by_idx ON users (referred_by) WHERE referred_by IS NOT NULL;
-- Награда за приглашённого: не больше одной строки на приглашённого за всё время (PRIMARY KEY).
-- Строка создаётся в транзакции первого подтверждённого платежа друга; referred_rewarded_at — друг получил дни в той же
-- транзакции (NULL у заблокированного), referrer_rewarded_at — пригласивший получил дни отдельной транзакцией позже.
CREATE TABLE referral_rewards (
    referred_id          bigint PRIMARY KEY,
    referrer_id          bigint NOT NULL,
    days                 int NOT NULL CHECK (days BETWEEN 1 AND 365),
    payment_id           bigint NOT NULL,
    referred_rewarded_at timestamptz,
    referrer_rewarded_at timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX referral_rewards_referrer_idx ON referral_rewards (referrer_id);
CREATE INDEX referral_rewards_due_idx ON referral_rewards (created_at) WHERE referrer_rewarded_at IS NULL;
-- +goose Down
DROP TABLE referral_rewards;
DROP INDEX users_referred_by_idx;
ALTER TABLE users DROP COLUMN referred_by;

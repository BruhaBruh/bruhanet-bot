-- +goose Up
-- Пробный период: один раз на юзера, время выдачи. Считается неоплаченным авансом, пока нет подтверждённой подписки.
ALTER TABLE users ADD COLUMN trial_used_at timestamptz;
CREATE INDEX users_trial_idx ON users (trial_used_at) WHERE trial_used_at IS NOT NULL;

-- Источник выдачи дней: admin — из панели, trial — пробный (by_admin_id пуст); дальше referral, gift.
ALTER TABLE grants
    ADD COLUMN source text NOT NULL DEFAULT 'admin' CHECK (source IN ('admin', 'trial', 'referral', 'gift')),
    ALTER COLUMN by_admin_id DROP NOT NULL;

-- +goose Down
ALTER TABLE grants DROP COLUMN source;
DELETE FROM grants WHERE by_admin_id IS NULL;
ALTER TABLE grants ALTER COLUMN by_admin_id SET NOT NULL;
DROP INDEX users_trial_idx;
ALTER TABLE users DROP COLUMN trial_used_at;

-- +goose Up
ALTER TABLE broadcasts
    ADD COLUMN status        text NOT NULL DEFAULT 'sending'
        CONSTRAINT broadcasts_status_check CHECK (status IN ('draft', 'scheduled', 'sending', 'sent', 'cancelled')),
    ADD COLUMN scheduled_at  timestamptz,
    ADD COLUMN audience      text NOT NULL DEFAULT 'all'
        CONSTRAINT broadcasts_audience_check CHECK (audience IN ('all', 'active', 'no_subscription', 'expiring')),
    ADD COLUMN audience_days int CONSTRAINT broadcasts_audience_days_check CHECK (audience_days BETWEEN 1 AND 60),
    ADD COLUMN silent        bool NOT NULL DEFAULT false,
    ADD COLUMN buttons       jsonb NOT NULL DEFAULT '[]',
    ADD COLUMN updated_at    timestamptz NOT NULL DEFAULT now();

UPDATE broadcasts SET status = 'sent' WHERE finished_at IS NOT NULL;
UPDATE broadcasts SET updated_at = COALESCE(finished_at, created_at);
ALTER TABLE broadcasts ALTER COLUMN status DROP DEFAULT;

-- Черновик живёт без ключа идемпотентности (его выдаёт отправка) и может быть пустым.
ALTER TABLE broadcasts ALTER COLUMN idempotency_key DROP NOT NULL;
ALTER TABLE broadcasts
    ADD CONSTRAINT broadcasts_key_check CHECK (status = 'draft' OR status = 'cancelled' OR idempotency_key IS NOT NULL),
    ADD CONSTRAINT broadcasts_content_check CHECK (status IN ('draft', 'cancelled') OR text <> '' OR photo IS NOT NULL);

CREATE INDEX broadcasts_scheduled_idx ON broadcasts (scheduled_at) WHERE status = 'scheduled';

-- +goose Down
DROP INDEX broadcasts_scheduled_idx;
DELETE FROM broadcasts WHERE idempotency_key IS NULL;
ALTER TABLE broadcasts
    DROP CONSTRAINT broadcasts_content_check,
    DROP CONSTRAINT broadcasts_key_check;
ALTER TABLE broadcasts ALTER COLUMN idempotency_key SET NOT NULL;
ALTER TABLE broadcasts
    DROP COLUMN updated_at,
    DROP COLUMN buttons,
    DROP COLUMN silent,
    DROP COLUMN audience_days,
    DROP COLUMN audience,
    DROP COLUMN scheduled_at,
    DROP COLUMN status;

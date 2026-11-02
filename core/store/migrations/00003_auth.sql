-- +goose Up
CREATE TABLE login_tokens (
    token_hash bytea PRIMARY KEY,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);

CREATE TABLE sessions (
    id_hash     bytea PRIMARY KEY,
    telegram_id bigint NOT NULL,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE sessions;
DROP TABLE login_tokens;

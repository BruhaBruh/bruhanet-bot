-- +goose Up
CREATE TABLE texts (
    key        text PRIMARY KEY,
    body       text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE settings (
    key        text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE settings;
DROP TABLE texts;

-- +goose Up
CREATE TABLE users (
    id            bigserial PRIMARY KEY,
    email         text        NOT NULL UNIQUE CHECK (email = lower(btrim(email)) AND email <> ''),
    status        text        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'blocked')),
    is_admin      boolean     NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_login_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX users_status_idx ON users (status);

-- +goose Down
DROP TABLE users;

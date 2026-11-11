-- +goose Up
ALTER TABLE users ADD COLUMN display_name text NOT NULL DEFAULT '' CHECK (length(display_name) <= 80);

CREATE TABLE circles (
    id         bigserial PRIMARY KEY,
    name       text        NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 80),
    created_by bigint      REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE circle_members (
    circle_id bigint      NOT NULL REFERENCES circles (id) ON DELETE CASCADE,
    user_id   bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role      text        NOT NULL CHECK (role IN ('admin', 'member')),
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (circle_id, user_id)
);

CREATE INDEX circle_members_user_idx ON circle_members (user_id);

CREATE TABLE circle_invites (
    id         bigserial PRIMARY KEY,
    circle_id  bigint      NOT NULL REFERENCES circles (id) ON DELETE CASCADE,
    token      text        NOT NULL UNIQUE,
    created_by bigint      REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);

CREATE INDEX circle_invites_circle_idx ON circle_invites (circle_id);

-- A person is a gift list owner: a circle member (user_id set) or someone
-- without an account (user_id null, managed by guardians).
CREATE TABLE persons (
    id         bigserial PRIMARY KEY,
    circle_id  bigint      NOT NULL REFERENCES circles (id) ON DELETE CASCADE,
    name       text        NOT NULL DEFAULT '' CHECK (length(name) <= 80),
    user_id    bigint      REFERENCES users (id) ON DELETE CASCADE,
    created_by bigint      REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (circle_id, user_id),
    CHECK (user_id IS NOT NULL OR btrim(name) <> '')
);

CREATE INDEX persons_circle_idx ON persons (circle_id);

CREATE TABLE person_guardians (
    person_id bigint NOT NULL REFERENCES persons (id) ON DELETE CASCADE,
    user_id   bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (person_id, user_id)
);

-- +goose Down
DROP TABLE person_guardians;
DROP TABLE persons;
DROP TABLE circle_invites;
DROP TABLE circle_members;
DROP TABLE circles;
ALTER TABLE users DROP COLUMN display_name;

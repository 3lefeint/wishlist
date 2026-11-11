-- +goose Up
CREATE TABLE tags (
    id         bigserial PRIMARY KEY,
    circle_id  bigint NOT NULL REFERENCES circles (id) ON DELETE CASCADE,
    name       text   NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 40),
    color      text   NOT NULL CHECK (color ~ '^#[0-9a-f]{6}$'),
    text_color text   NOT NULL CHECK (text_color IN ('#000000', '#ffffff'))
);

CREATE UNIQUE INDEX tags_circle_name_idx ON tags (circle_id, lower(name));

CREATE TABLE gifts (
    id         bigserial PRIMARY KEY,
    circle_id  bigint      NOT NULL REFERENCES circles (id) ON DELETE CASCADE,
    person_id  bigint      NOT NULL REFERENCES persons (id) ON DELETE CASCADE,
    title      text        NOT NULL CHECK (btrim(title) <> '' AND length(title) <= 120),
    comment    text        NOT NULL DEFAULT '' CHECK (length(comment) <= 2000),
    url        text        NOT NULL DEFAULT '' CHECK (length(url) <= 2000),
    has_image  boolean     NOT NULL DEFAULT false,
    active     boolean     NOT NULL DEFAULT true,
    created_by bigint      REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX gifts_person_idx ON gifts (person_id);
CREATE INDEX gifts_circle_idx ON gifts (circle_id);

CREATE TABLE gift_tags (
    gift_id bigint NOT NULL REFERENCES gifts (id) ON DELETE CASCADE,
    tag_id  bigint NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (gift_id, tag_id)
);

CREATE TABLE claims (
    gift_id    bigint      PRIMARY KEY REFERENCES gifts (id) ON DELETE CASCADE,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX claims_user_idx ON claims (user_id);

-- +goose Down
DROP TABLE claims;
DROP TABLE gift_tags;
DROP TABLE gifts;
DROP TABLE tags;

-- +goose Up
CREATE TABLE households (
    id         bigserial PRIMARY KEY,
    circle_id  bigint      NOT NULL REFERENCES circles (id) ON DELETE CASCADE,
    name       text        NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 80),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, circle_id)
);

CREATE INDEX households_circle_idx ON households (circle_id);

-- Persons cover members with an account and people without one. A person belongs to at most one
-- household of its circle. Deleting a household clears the link first (done by the application),
-- because the composite key includes circle_id.
ALTER TABLE persons
    ADD COLUMN household_id bigint,
    ADD FOREIGN KEY (household_id, circle_id) REFERENCES households (id, circle_id);

-- +goose Down
ALTER TABLE persons DROP COLUMN household_id;
DROP TABLE households;

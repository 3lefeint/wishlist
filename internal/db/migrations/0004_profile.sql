-- +goose Up
ALTER TABLE users ADD COLUMN first_name text NOT NULL DEFAULT '' CHECK (length(first_name) <= 80);
ALTER TABLE users ADD COLUMN last_name text NOT NULL DEFAULT '' CHECK (length(last_name) <= 80);
ALTER TABLE users ADD COLUMN birthday date;
ALTER TABLE users ADD COLUMN favorite_color text NOT NULL DEFAULT '' CHECK (favorite_color = '' OR favorite_color ~ '^#[0-9a-f]{6}$');
UPDATE users SET first_name = display_name;
ALTER TABLE users DROP COLUMN display_name;
ALTER TABLE persons ADD COLUMN birthday date;

-- +goose Down
ALTER TABLE persons DROP COLUMN birthday;
ALTER TABLE users ADD COLUMN display_name text NOT NULL DEFAULT '' CHECK (length(display_name) <= 80);
UPDATE users SET display_name = left(btrim(first_name || ' ' || last_name), 80);
ALTER TABLE users DROP COLUMN favorite_color;
ALTER TABLE users DROP COLUMN birthday;
ALTER TABLE users DROP COLUMN last_name;
ALTER TABLE users DROP COLUMN first_name;

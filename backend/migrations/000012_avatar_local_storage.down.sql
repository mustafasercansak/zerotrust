ALTER TABLE users
    ADD COLUMN IF NOT EXISTS avatar_data BYTEA;

ALTER TABLE users
    DROP COLUMN IF EXISTS avatar_size,
    DROP COLUMN IF EXISTS avatar_object_key;

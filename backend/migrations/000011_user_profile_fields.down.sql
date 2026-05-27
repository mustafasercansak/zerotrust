ALTER TABLE users
    DROP COLUMN IF EXISTS avatar_updated_at,
    DROP COLUMN IF EXISTS avatar_size,
    DROP COLUMN IF EXISTS avatar_mime_type,
    DROP COLUMN IF EXISTS avatar_object_key,
    DROP COLUMN IF EXISTS last_name,
    DROP COLUMN IF EXISTS first_name;

ALTER TABLE users
    ALTER COLUMN email_hash DROP NOT NULL;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM users
        WHERE length(email) > 255
           OR length(first_name) > 80
           OR length(last_name) > 80
    ) THEN
        RAISE EXCEPTION 'cannot restore user field length limits while expanded ciphertext exists';
    END IF;
END
$$;

ALTER TABLE users
    ALTER COLUMN email TYPE VARCHAR(255) USING email::VARCHAR(255),
    ALTER COLUMN first_name TYPE VARCHAR(80) USING first_name::VARCHAR(80),
    ALTER COLUMN last_name TYPE VARCHAR(80) USING last_name::VARCHAR(80);

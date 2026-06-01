DROP INDEX IF EXISTS idx_users_email_hash;
ALTER TABLE users DROP COLUMN IF EXISTS email_hash;

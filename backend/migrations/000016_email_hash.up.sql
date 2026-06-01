ALTER TABLE users ADD COLUMN IF NOT EXISTS email_hash VARCHAR(64) UNIQUE;
CREATE INDEX IF NOT EXISTS idx_users_email_hash ON users(email_hash);

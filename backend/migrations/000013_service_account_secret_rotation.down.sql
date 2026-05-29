ALTER TABLE service_accounts DROP COLUMN IF EXISTS old_client_secret_hash;
ALTER TABLE service_accounts DROP COLUMN IF EXISTS old_secret_expires_at;

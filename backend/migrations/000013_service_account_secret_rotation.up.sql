ALTER TABLE service_accounts ADD COLUMN old_client_secret_hash VARCHAR(255);
ALTER TABLE service_accounts ADD COLUMN old_secret_expires_at TIMESTAMPTZ;

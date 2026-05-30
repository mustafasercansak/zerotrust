ALTER TABLE user_mfa ADD COLUMN recovery_codes TEXT[];
ALTER TABLE user_mfa ADD COLUMN pending_recovery_codes TEXT[];

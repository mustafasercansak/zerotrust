ALTER TABLE user_mfa DROP COLUMN IF EXISTS recovery_codes;
ALTER TABLE user_mfa DROP COLUMN IF EXISTS pending_recovery_codes;

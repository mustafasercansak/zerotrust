DELETE FROM system_settings WHERE key IN ('password_complexity', 'global_mfa_required', 'max_login_attempts');

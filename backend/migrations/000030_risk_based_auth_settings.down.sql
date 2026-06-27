DELETE FROM system_settings WHERE key IN (
  'risk_based_auth_enabled',
  'risk_threshold_mfa',
  'risk_threshold_block'
);

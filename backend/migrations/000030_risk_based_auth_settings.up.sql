INSERT INTO system_settings (key, value) VALUES
  ('risk_based_auth_enabled', 'false'),
  ('risk_threshold_mfa', '40'),
  ('risk_threshold_block', '80')
ON CONFLICT (key) DO NOTHING;

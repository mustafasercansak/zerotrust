DELETE FROM system_settings WHERE key IN (
  'risk_score_impossible_travel',
  'risk_score_new_device',
  'risk_score_suspicious_hours',
  'risk_score_failed_attempt',
  'risk_failed_attempt_max_score',
  'risk_suspicious_hour_start',
  'risk_suspicious_hour_end',
  'risk_impossible_travel_velocity_kmh',
  'risk_impossible_travel_window_hours',
  'risk_impossible_travel_min_distance_km'
);

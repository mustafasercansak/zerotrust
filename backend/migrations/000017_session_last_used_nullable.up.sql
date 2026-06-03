-- Make last_used_at nullable so RevokeStaleInitialSessions can identify
-- sessions that were created but never refreshed (last_used_at IS NULL).
-- All COALESCE(last_used_at, created_at) callers already handle NULL safely.
ALTER TABLE sessions ALTER COLUMN last_used_at DROP NOT NULL;
ALTER TABLE sessions ALTER COLUMN last_used_at DROP DEFAULT;

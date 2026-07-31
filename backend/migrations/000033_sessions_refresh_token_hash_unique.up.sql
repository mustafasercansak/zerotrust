-- Fail loudly if duplicate refresh_token_hash values already exist; they must
-- be resolved manually before the unique index can be created.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM sessions
        GROUP BY refresh_token_hash
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'sessions.refresh_token_hash contains duplicate values; resolve them before applying the unique index';
    END IF;
END $$;

-- Replace the plain index with a unique one (same name, now enforcing uniqueness).
DROP INDEX idx_sessions_token_hash;
CREATE UNIQUE INDEX idx_sessions_token_hash ON sessions(refresh_token_hash);

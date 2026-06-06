-- Repeat the idempotent backfill for databases that previously applied an
-- earlier version of migration 20, then enforce the repository invariant.
UPDATE users
SET email_hash = encode(digest(lower(btrim(email)), 'sha256'), 'hex')
WHERE email_hash IS NULL
  AND email !~ '^vault:v[0-9]+:';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE email_hash IS NULL) THEN
        RAISE EXCEPTION 'cannot enforce email_hash: users with missing hashes exist';
    END IF;
END
$$;

ALTER TABLE users
    ALTER COLUMN email_hash SET NOT NULL;

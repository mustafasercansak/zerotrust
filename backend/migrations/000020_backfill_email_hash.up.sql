DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM users
        WHERE email_hash IS NULL
          AND email !~ '^vault:v[0-9]+:'
        GROUP BY lower(btrim(email))
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'cannot backfill email_hash: duplicate normalized email addresses exist';
    END IF;
END
$$;

UPDATE users
SET email_hash = encode(digest(lower(btrim(email)), 'sha256'), 'hex')
WHERE email_hash IS NULL
  AND email !~ '^vault:v[0-9]+:';

-- Remove the demo OAuth2 client from any environment where it still exists.
-- Migration 000027 removed it going forward, and its down-migration no longer
-- re-seeds it (the bcrypt hash's plaintext secret is published in git history,
-- making the client a fully usable credential for anyone). Environments that
-- ran the OLD 000027 down-migration before that change still have the row;
-- this deletes it everywhere, idempotently. (#108)
DELETE FROM oauth2_clients WHERE client_id = 'demo-client';

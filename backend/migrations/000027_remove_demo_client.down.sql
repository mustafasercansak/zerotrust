-- No-op: this down-migration intentionally does NOT re-seed the demo OAuth2
-- client. The original version re-inserted 'demo-client' with a bcrypt hash
-- whose plaintext secret is published in the git history, so a rollback would
-- have silently restored a fully usable OAuth2 client with a publicly known
-- credential (#108). If a demo client is needed for local development, create
-- one through the admin API (or seed it with a freshly generated secret).
SELECT 1;

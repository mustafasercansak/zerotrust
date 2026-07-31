-- No-op: intentionally does NOT restore the demo OAuth2 client. Its client
-- secret is publicly known (see 000027 down-migration notes). If a demo client
-- is needed for local development, create one through the admin API with a
-- freshly generated secret.
SELECT 1;

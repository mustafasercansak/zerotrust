-- Remove the development demo-client seeded in migration 000023.
-- That client has a publicly known secret and must not be present in production.
DELETE FROM oauth2_clients WHERE client_id = 'demo-client';

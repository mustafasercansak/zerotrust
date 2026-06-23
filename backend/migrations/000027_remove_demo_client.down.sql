-- Restore the demo-client for development environments only.
-- Do NOT run this down-migration on production.
INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
VALUES (
    'demo-client',
    '$2a$12$Xi8n6yn8qXuyeVdGn6wfU.Vw2BVtMd/78bk/76tzxeqDvxpJaJEY6',
    'Demo OAuth2 Client',
    ARRAY['http://localhost:3000/callback', 'https://oauth.pstmn.io/v1/callback'],
    ARRAY['openid', 'profile', 'email']
) ON CONFLICT (client_id) DO NOTHING;

CREATE TABLE oauth2_clients (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id           VARCHAR(255) NOT NULL UNIQUE,
    client_secret_hash  VARCHAR(255) NOT NULL,
    name                VARCHAR(255) NOT NULL,
    redirect_uris       TEXT[] NOT NULL,
    allowed_scopes      TEXT[] NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed a demo OAuth2 Client for development/testing
-- Secret is "demo-secret"
INSERT INTO oauth2_clients (client_id, client_secret_hash, name, redirect_uris, allowed_scopes)
VALUES (
    'demo-client',
    '$2a$12$Xi8n6yn8qXuyeVdGn6wfU.Vw2BVtMd/78bk/76tzxeqDvxpJaJEY6',
    'Demo OAuth2 Client',
    ARRAY['http://localhost:3000/callback', 'https://oauth.pstmn.io/v1/callback'],
    ARRAY['openid', 'profile', 'email']
);

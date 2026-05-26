-- Fine-grained permissions
CREATE TABLE permissions (
    id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource VARCHAR(64) NOT NULL,
    action   VARCHAR(32) NOT NULL,
    UNIQUE (resource, action)
);

CREATE TABLE role_permissions (
    role_id       UUID NOT NULL REFERENCES roles(id)       ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- Service accounts (machine-to-machine)
CREATE TABLE service_accounts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name               VARCHAR(128) NOT NULL UNIQUE,
    client_id          VARCHAR(64)  NOT NULL UNIQUE,
    client_secret_hash VARCHAR(255) NOT NULL,
    is_active          BOOLEAN NOT NULL DEFAULT true,
    created_by         UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE service_account_scopes (
    service_account_id UUID    NOT NULL REFERENCES service_accounts(id) ON DELETE CASCADE,
    scope              VARCHAR(64) NOT NULL,
    PRIMARY KEY (service_account_id, scope)
);

-- Seed permissions
INSERT INTO permissions (resource, action) VALUES
    ('users',            'read'),
    ('users',            'write'),
    ('users',            'delete'),
    ('audit',            'read'),
    ('service_accounts', 'read'),
    ('service_accounts', 'write'),
    ('service_accounts', 'delete'),
    ('tokens',           'validate');

-- Admin role gets all permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM roles r CROSS JOIN permissions p WHERE r.name = 'admin';

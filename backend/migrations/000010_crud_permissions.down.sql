-- Restore the previous broad write permissions.
INSERT INTO permissions (resource, action) VALUES
    ('users', 'write'),
    ('service_accounts', 'write')
ON CONFLICT (resource, action) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'admin'
  AND p.action = 'write'
  AND p.resource IN ('users', 'service_accounts')
ON CONFLICT DO NOTHING;

INSERT INTO service_account_scopes (service_account_id, scope)
SELECT DISTINCT service_account_id, 'users:write'
FROM service_account_scopes
WHERE scope IN ('users:create', 'users:update')
ON CONFLICT DO NOTHING;

INSERT INTO service_account_scopes (service_account_id, scope)
SELECT DISTINCT service_account_id, 'service_accounts:write'
FROM service_account_scopes
WHERE scope IN ('service_accounts:create', 'service_accounts:update')
ON CONFLICT DO NOTHING;

DELETE FROM service_account_scopes
WHERE scope IN ('users:create', 'users:update', 'service_accounts:create', 'service_accounts:update');

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.action IN ('create', 'update')
  AND p.resource IN ('users', 'service_accounts');

DELETE FROM permissions
WHERE action IN ('create', 'update')
  AND resource IN ('users', 'service_accounts');

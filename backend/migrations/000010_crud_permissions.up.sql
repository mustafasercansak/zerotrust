-- Split broad write permissions into explicit CRUD-style actions.
INSERT INTO permissions (resource, action) VALUES
    ('users', 'create'),
    ('users', 'update'),
    ('service_accounts', 'create'),
    ('service_accounts', 'update')
ON CONFLICT (resource, action) DO NOTHING;

-- Preserve any role that previously had write by granting create and update.
INSERT INTO role_permissions (role_id, permission_id)
SELECT DISTINCT rp.role_id, replacement.id
FROM role_permissions rp
JOIN permissions legacy ON legacy.id = rp.permission_id
JOIN permissions replacement
  ON replacement.resource = legacy.resource
 AND replacement.action IN ('create', 'update')
WHERE legacy.action = 'write'
  AND legacy.resource IN ('users', 'service_accounts')
ON CONFLICT DO NOTHING;

-- Admin should have the new permissions even on databases without legacy write rows.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'admin'
  AND (
    (p.resource = 'users' AND p.action IN ('create', 'update'))
    OR (p.resource = 'service_accounts' AND p.action IN ('create', 'update'))
  )
ON CONFLICT DO NOTHING;

-- Existing service accounts that had write can create and update after the split.
INSERT INTO service_account_scopes (service_account_id, scope)
SELECT service_account_id, 'users:create'
FROM service_account_scopes
WHERE scope = 'users:write'
ON CONFLICT DO NOTHING;

INSERT INTO service_account_scopes (service_account_id, scope)
SELECT service_account_id, 'users:update'
FROM service_account_scopes
WHERE scope = 'users:write'
ON CONFLICT DO NOTHING;

INSERT INTO service_account_scopes (service_account_id, scope)
SELECT service_account_id, 'service_accounts:create'
FROM service_account_scopes
WHERE scope = 'service_accounts:write'
ON CONFLICT DO NOTHING;

INSERT INTO service_account_scopes (service_account_id, scope)
SELECT service_account_id, 'service_accounts:update'
FROM service_account_scopes
WHERE scope = 'service_accounts:write'
ON CONFLICT DO NOTHING;

DELETE FROM service_account_scopes
WHERE scope IN ('users:write', 'service_accounts:write');

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.action = 'write'
  AND p.resource IN ('users', 'service_accounts');

DELETE FROM permissions
WHERE action = 'write'
  AND resource IN ('users', 'service_accounts');

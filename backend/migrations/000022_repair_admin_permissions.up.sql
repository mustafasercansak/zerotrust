-- Roles and role permissions are application configuration, not test data.
-- Repair installations where destructive test cleanup or an incomplete seed
-- removed the canonical roles or the admin permission mappings.
INSERT INTO roles (name, description) VALUES
    ('admin', 'Tam yetkili sistem yöneticisi'),
    ('user', 'Standart kullanıcı')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'admin'
ON CONFLICT DO NOTHING;

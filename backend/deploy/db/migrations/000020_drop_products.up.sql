-- The products example module is gone: its table, its RBAC resource and the
-- role permissions that named it.
DELETE FROM usr.usr_role_permission WHERE resource = 'products';
DELETE FROM usr.usr_resource WHERE code = 'products';
DROP TABLE IF EXISTS app.product;

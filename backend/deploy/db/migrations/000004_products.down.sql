DELETE FROM usr.usr_role_permission WHERE resource = 'products';
DELETE FROM usr.usr_resource WHERE code = 'products';
DROP TABLE app.product;

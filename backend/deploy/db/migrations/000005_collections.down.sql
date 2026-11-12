DELETE FROM usr.usr_role_permission WHERE resource = 'collections';
DELETE FROM usr.usr_resource WHERE code = 'collections';
DROP TABLE app.collection;

DELETE FROM usr.usr_role_permission WHERE resource = 'records';
DELETE FROM usr.usr_resource WHERE code = 'records';
DROP TABLE app.record;

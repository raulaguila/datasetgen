UPDATE usr.usr_role_permission SET actions = actions & ~1
WHERE resource = 'collections' AND role_id = (SELECT id FROM usr.usr_role WHERE name = 'user');
DELETE FROM usr.usr_role_permission
WHERE resource = 'collections' AND actions = 0 AND role_id = (SELECT id FROM usr.usr_role WHERE name = 'user');

-- Every account may start its own collections: the default "user" role gets
-- collections:create (bit 1). Membership governs everything else.
INSERT INTO usr.usr_role_permission (role_id, resource, actions)
SELECT id, 'collections', 1 FROM usr.usr_role WHERE name = 'user'
ON CONFLICT (role_id, resource) DO UPDATE SET actions = usr.usr_role_permission.actions | 1;

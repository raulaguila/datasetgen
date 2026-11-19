DROP VIEW IF EXISTS usr.vw_user_effective_permissions;
DROP TABLE IF EXISTS usr.usr_notification, usr.usr_preference, usr.usr_user_role, usr.usr_role_permission,
    usr.usr_role, usr.usr_login_attempt, usr.usr_token, usr.usr_session, usr.usr_recovery_code, usr.usr_mfa,
    usr.usr_credential, usr.usr_user;
DROP FUNCTION IF EXISTS usr.fn_user_username(), usr.fn_free_username(TEXT, UUID), usr.fn_username_reserved(TEXT);

DROP TRIGGER IF EXISTS trg_usr_user_username_moved ON usr.usr_user;
DROP FUNCTION IF EXISTS app.fn_purge_trash(INTERVAL);
DROP TABLE IF EXISTS app.import_batch, app.record_comment, app.file_gone, app.file, app.saved_view,
    app.collection_dashboard, app.collection_quality, app.collection_schema_version, app.record_trash,
    app.record_version, app.record, app.collection_path, app.collection_member, app.collection;
DROP FUNCTION IF EXISTS app.fn_file_gone(), app.fn_user_username_moved(), app.fn_collection_path(),
    app.fn_remember_path(TEXT, TEXT, UUID), app.fn_free_slug(UUID, TEXT, UUID);

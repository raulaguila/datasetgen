-- Comments on a record: a note any member can leave, removable by its author or an owner.
CREATE TABLE app.record_comment (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    record_id  UUID        NOT NULL REFERENCES app.record (id) ON DELETE CASCADE,
    user_id    UUID        REFERENCES usr.usr_user (id) ON DELETE SET NULL,
    body       TEXT        NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_app_record_comment_record ON app.record_comment (record_id, created_at);

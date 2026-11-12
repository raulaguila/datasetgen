-- Who works on each collection. Access to a collection and its records comes from
-- this table, not from RBAC: owner manages fields, settings and members; editor
-- writes records; viewer reads. A platform admin (superuser role or '*') sees all.
-- A member may be a pending account: inviting by email creates it and adds it here.

CREATE TABLE app.collection_member (
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    user_id       UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    role          TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (collection_id, user_id),
    CONSTRAINT chk_app_collection_member_role CHECK (role IN ('owner', 'editor', 'viewer'))
);

-- The sidebar lists a person's collections.
CREATE INDEX idx_app_collection_member_user ON app.collection_member (user_id);

-- Names were unique across the instance; with collections private to their
-- members, a hidden collection must neither block a name nor reveal it exists.
ALTER TABLE app.collection DROP CONSTRAINT uq_app_collection_name;
CREATE INDEX idx_app_collection_name ON app.collection (name, id);

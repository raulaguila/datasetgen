-- A collection's dashboard: the owners' chosen widgets (counts, sums, charts, lists), shown to every member.
CREATE TABLE app.collection_dashboard (
    collection_id UUID        PRIMARY KEY REFERENCES app.collection (id) ON DELETE CASCADE,
    widgets       JSON        NOT NULL DEFAULT '[]',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by    UUID        REFERENCES usr.usr_user (id) ON DELETE SET NULL
);

-- Automations: when a record is created or changed (or once a day), records
-- that pass the conditions get an action: set a field, or notify the owner.
CREATE TABLE app.automation (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    name          TEXT        NOT NULL,
    enabled       BOOLEAN     NOT NULL DEFAULT true,
    trigger       TEXT        NOT NULL CHECK (trigger IN ('record.created', 'record.updated', 'daily')),
    -- The daily run's hour (server time); NULL for record triggers.
    hour          SMALLINT    CHECK (hour BETWEEN 0 AND 23),
    conditions    JSONB       NOT NULL DEFAULT '{"any": false, "rules": []}',
    action        TEXT        NOT NULL CHECK (action IN ('set_field', 'notify')),
    config        JSONB       NOT NULL,
    created_by    UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_run_at   TIMESTAMPTZ,
    last_error    TEXT        NOT NULL DEFAULT '',
    runs          INTEGER     NOT NULL DEFAULT 0
);
CREATE INDEX idx_app_automation_collection ON app.automation (collection_id, trigger) WHERE enabled;

-- Each webhook call and its answer, newest 50 per hook, so a failed one can be sent again.
CREATE TABLE app.webhook_delivery (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id  UUID        NOT NULL REFERENCES app.webhook (id) ON DELETE CASCADE,
    event       TEXT        NOT NULL,
    body        JSONB       NOT NULL,
    status      INTEGER,
    error       TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_app_webhook_delivery_hook ON app.webhook_delivery (webhook_id, created_at DESC);

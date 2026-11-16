-- Tools a collection owner adds to the MCP server: a described, read-only search
-- with fixed conditions, typed parameters bound to fields, and chosen fields back.
CREATE TABLE app.mcp_tool (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    collection_id UUID        NOT NULL REFERENCES app.collection (id) ON DELETE CASCADE,
    name          TEXT        NOT NULL CHECK (name ~ '^[a-z][a-z0-9_]{2,47}$'),
    description   TEXT        NOT NULL,
    enabled       BOOLEAN     NOT NULL DEFAULT true,
    conditions    JSONB       NOT NULL DEFAULT '{"any": false, "rules": []}',
    params        JSONB       NOT NULL DEFAULT '[]',
    fields        JSONB       NOT NULL DEFAULT '[]',
    sort          TEXT        NOT NULL DEFAULT 'updated_at',
    sort_order    TEXT        NOT NULL DEFAULT 'desc' CHECK (sort_order IN ('asc', 'desc')),
    max_records   SMALLINT    NOT NULL DEFAULT 20 CHECK (max_records BETWEEN 1 AND 50),
    created_by    UUID        NOT NULL REFERENCES usr.usr_user (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (collection_id, name)
);

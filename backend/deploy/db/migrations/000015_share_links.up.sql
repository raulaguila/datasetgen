-- Share links: a public form that adds records, or a read-only list. A link is
-- an API key with a kind; its token is sealed (AES-GCM) so the owner can copy it again.
ALTER TABLE app.api_key
    ADD COLUMN share  TEXT CHECK (share IN ('form', 'view')),
    ADD COLUMN sealed TEXT;
ALTER TABLE app.api_key ADD CONSTRAINT api_key_share_sealed CHECK ((share IS NULL) = (sealed IS NULL));

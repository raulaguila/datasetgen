-- Ids of deleted file rows whose stored bytes are still to be removed. The
-- trigger also catches rows a collection purge cascades away.
CREATE TABLE app.file_gone (
    id         UUID        PRIMARY KEY,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE FUNCTION app.fn_file_gone() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO app.file_gone (id) VALUES (OLD.id) ON CONFLICT DO NOTHING;
    RETURN OLD;
END $$;

CREATE TRIGGER trg_file_gone AFTER DELETE ON app.file FOR EACH ROW EXECUTE FUNCTION app.fn_file_gone();

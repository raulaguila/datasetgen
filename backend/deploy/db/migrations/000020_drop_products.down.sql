-- Brings back the empty products table and resource of 000004 (roles get no permission back).
CREATE TABLE app.product (
    id         UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT           NOT NULL,
    price      NUMERIC(12, 2) NOT NULL,
    enabled    BOOLEAN        NOT NULL DEFAULT true,
    -- Optimistic concurrency: every update bumps it, and an update that names the
    -- version it read is refused when someone else changed the row since.
    version    INTEGER        NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ    NOT NULL DEFAULT now(),
    CONSTRAINT chk_app_product_name_len CHECK (char_length(name) BETWEEN 1 AND 120),
    CONSTRAINT chk_app_product_price_nonneg CHECK (price >= 0)
);

CREATE INDEX idx_app_product_name ON app.product (name);
CREATE INDEX idx_app_product_name_trgm ON app.product USING gin (name gin_trgm_ops);
CREATE INDEX idx_app_product_cursor ON app.product (created_at DESC, id DESC);
CREATE INDEX idx_app_product_price ON app.product (price, id);

CREATE TRIGGER trg_app_product_set_updated_at BEFORE UPDATE ON app.product
    FOR EACH ROW EXECUTE FUNCTION public.fn_set_updated_at();

INSERT INTO usr.usr_resource (code, description) VALUES ('products', 'Example CRUD module');

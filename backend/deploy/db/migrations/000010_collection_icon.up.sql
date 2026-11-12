-- A collection may show a curated icon instead of its initials ('' = initials).
-- The API accepts only the slugs the frontend ships (domain.ValidCollectionIcon).
ALTER TABLE app.collection ADD COLUMN icon TEXT NOT NULL DEFAULT '';

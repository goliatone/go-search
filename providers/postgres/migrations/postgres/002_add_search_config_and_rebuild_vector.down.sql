DROP INDEX IF EXISTS idx_search_documents_search_vector;
ALTER TABLE search_documents DROP COLUMN IF EXISTS search_vector;
ALTER TABLE search_documents DROP COLUMN IF EXISTS search_config;

ALTER TABLE search_documents
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(summary, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(body, '')), 'C')
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_search_documents_search_vector ON search_documents USING GIN (search_vector);

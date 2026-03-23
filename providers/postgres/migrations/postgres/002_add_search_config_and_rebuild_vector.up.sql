ALTER TABLE search_documents
    ADD COLUMN IF NOT EXISTS search_config TEXT NOT NULL DEFAULT 'simple';

UPDATE search_documents
SET search_config = 'simple'
WHERE COALESCE(search_config, '') = '';

DROP INDEX IF EXISTS idx_search_documents_search_vector;
ALTER TABLE search_documents DROP COLUMN IF EXISTS search_vector;

ALTER TABLE search_documents
    ADD COLUMN search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector(search_config::regconfig, coalesce(title, '')), 'A') ||
        setweight(to_tsvector(search_config::regconfig, coalesce(summary, '')), 'B') ||
        setweight(to_tsvector(search_config::regconfig, coalesce(body, '')), 'C')
    ) STORED;

CREATE INDEX IF NOT EXISTS idx_search_documents_search_vector ON search_documents USING GIN (search_vector);

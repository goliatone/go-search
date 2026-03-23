ALTER TABLE search_documents
    ADD COLUMN IF NOT EXISTS registration_key TEXT NOT NULL DEFAULT '';

UPDATE search_documents
SET registration_key = ''
WHERE COALESCE(registration_key, '') = '';

DROP INDEX IF EXISTS idx_search_documents_source_id;

ALTER TABLE search_documents
    DROP CONSTRAINT IF EXISTS search_documents_pkey;

ALTER TABLE search_documents
    ADD CONSTRAINT search_documents_pkey PRIMARY KEY (index_name, registration_key, document_id);

CREATE INDEX IF NOT EXISTS idx_search_documents_source_id
    ON search_documents (index_name, registration_key, source_id);

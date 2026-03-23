DROP INDEX IF EXISTS idx_search_documents_source_id;

ALTER TABLE search_documents
    DROP CONSTRAINT IF EXISTS search_documents_pkey;

ALTER TABLE search_documents
    ADD CONSTRAINT search_documents_pkey PRIMARY KEY (index_name, document_id);

CREATE INDEX IF NOT EXISTS idx_search_documents_source_id
    ON search_documents (index_name, source_id);

ALTER TABLE search_documents
    DROP COLUMN IF EXISTS registration_key;

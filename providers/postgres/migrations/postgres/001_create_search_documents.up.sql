CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS search_documents (
    index_name TEXT NOT NULL,
    registration_key TEXT NOT NULL DEFAULT '',
    document_id TEXT NOT NULL,
    document_type TEXT NOT NULL DEFAULT '',
    parent_id TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    source_version TEXT NOT NULL DEFAULT '',
    search_config TEXT NOT NULL DEFAULT 'simple',
    title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    url TEXT NOT NULL DEFAULT '',
    anchor_url TEXT NOT NULL DEFAULT '',
    locale TEXT NOT NULL DEFAULT '',
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at_unix BIGINT,
    updated_at_unix BIGINT,
    published_at_unix BIGINT,
    start_ms BIGINT,
    end_ms BIGINT,
    fields JSONB NOT NULL DEFAULT '{}'::jsonb,
    facets JSONB NOT NULL DEFAULT '{}'::jsonb,
    numeric JSONB NOT NULL DEFAULT '{}'::jsonb,
    booleans JSONB NOT NULL DEFAULT '{}'::jsonb,
    scope_tenant_id TEXT NOT NULL DEFAULT '',
    scope_org_id TEXT NOT NULL DEFAULT '',
    scope_labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    visibility_public BOOLEAN NOT NULL DEFAULT FALSE,
    visibility_roles TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    visibility_permissions TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    visibility_status TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    searchable_text TEXT GENERATED ALWAYS AS (
        trim(
            BOTH ' '
            FROM concat_ws(
                ' ',
                coalesce(title, ''),
                coalesce(summary, ''),
                coalesce(body, '')
            )
        )
    ) STORED,
    search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector(search_config::regconfig, coalesce(title, '')), 'A') ||
        setweight(to_tsvector(search_config::regconfig, coalesce(summary, '')), 'B') ||
        setweight(to_tsvector(search_config::regconfig, coalesce(body, '')), 'C')
    ) STORED,
    PRIMARY KEY (index_name, registration_key, document_id)
);

CREATE INDEX IF NOT EXISTS idx_search_documents_index_name ON search_documents (index_name);
CREATE INDEX IF NOT EXISTS idx_search_documents_source_id ON search_documents (index_name, registration_key, source_id);
CREATE INDEX IF NOT EXISTS idx_search_documents_parent_id ON search_documents (index_name, parent_id);
CREATE INDEX IF NOT EXISTS idx_search_documents_locale ON search_documents (index_name, locale);
CREATE INDEX IF NOT EXISTS idx_search_documents_search_vector ON search_documents USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_search_documents_title_trgm ON search_documents USING GIN (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_search_documents_summary_trgm ON search_documents USING GIN (summary gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_search_documents_body_trgm ON search_documents USING GIN (body gin_trgm_ops);

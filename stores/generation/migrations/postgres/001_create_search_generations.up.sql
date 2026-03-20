CREATE TABLE IF NOT EXISTS search_generations (
    index_name TEXT PRIMARY KEY,
    generation BIGINT NOT NULL DEFAULT 0,
    last_indexed_at BIGINT NOT NULL DEFAULT 0
);

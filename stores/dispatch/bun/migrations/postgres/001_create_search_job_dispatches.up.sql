CREATE TABLE IF NOT EXISTS search_job_dispatches (
    dispatch_id TEXT PRIMARY KEY,
    operation_key TEXT NOT NULL UNIQUE,
    batch_id TEXT NOT NULL DEFAULT '',
    batch_position INTEGER NOT NULL DEFAULT 0,
    state TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    snapshot JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_search_job_dispatches_batch
    ON search_job_dispatches (batch_id, batch_position, dispatch_id);

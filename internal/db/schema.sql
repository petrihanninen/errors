CREATE TABLE IF NOT EXISTS error_groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    message TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'todo',
    occurrences INTEGER NOT NULL DEFAULT 0,
    first_seen BIGINT,
    last_seen BIGINT,
    events_query TEXT,
    link TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_error_groups_status ON error_groups(status);

CREATE TABLE IF NOT EXISTS error_occurrences (
    id SERIAL PRIMARY KEY,
    error_group_id TEXT NOT NULL REFERENCES error_groups(id),
    error_class TEXT,
    message TEXT,
    host TEXT,
    request_uri TEXT,
    transaction_name TEXT,
    occurred_at BIGINT NOT NULL,
    UNIQUE(error_group_id, occurred_at, host)
);

CREATE INDEX IF NOT EXISTS idx_error_occurrences_group ON error_occurrences(error_group_id);

CREATE TABLE IF NOT EXISTS fix_attempts (
    id SERIAL PRIMARY KEY,
    error_group_id TEXT NOT NULL REFERENCES error_groups(id),
    branch_name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'running',
    agent_output TEXT,
    commit_sha TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_fix_attempts_group ON fix_attempts(error_group_id);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

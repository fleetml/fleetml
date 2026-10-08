-- Dashboard sessions: an API key exchanged for an httpOnly cookie.
CREATE TABLE IF NOT EXISTS sessions (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    api_key_id    UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    token_hash    CHAR(64) NOT NULL UNIQUE,
    user_agent    TEXT NOT NULL DEFAULT '',
    ip            VARCHAR(64) NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ NOT NULL,
    revoked_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

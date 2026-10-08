-- Admin API keys for the REST API. Only the sha256 of each key is stored.
CREATE TABLE IF NOT EXISTS api_keys (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name          VARCHAR(255) NOT NULL,
    key_hash      CHAR(64) NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at  TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ
);

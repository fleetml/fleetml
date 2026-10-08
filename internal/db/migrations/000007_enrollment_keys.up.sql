-- Per-project enrollment keys: a device must present one to register.
-- Only the sha256 of each key is stored.
CREATE TABLE IF NOT EXISTS enrollment_keys (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id    UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name          VARCHAR(255) NOT NULL,
    key_hash      CHAR(64) NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at  TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_enrollment_keys_project ON enrollment_keys(project_id);

-- The project a device enrolled into (NULL for devices registered before enrollment keys).
ALTER TABLE devices ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES projects(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_devices_project ON devices(project_id);

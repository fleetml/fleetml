-- Users own API keys and hold a role per project (viewer, operator, admin);
-- a global admin may do everything. Existing keys become keys of the
-- "bootstrap" user, a global admin.
CREATE TABLE IF NOT EXISTS users (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(100) NOT NULL UNIQUE,
    is_admin    BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO users (id, name, is_admin) VALUES ('00000000-0000-4000-8000-000000000001', 'bootstrap', true)
    ON CONFLICT DO NOTHING;

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE CASCADE;
UPDATE api_keys SET user_id = '00000000-0000-4000-8000-000000000001' WHERE user_id IS NULL;
ALTER TABLE api_keys ALTER COLUMN user_id SET NOT NULL;

CREATE TABLE IF NOT EXISTS project_roles (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    role        VARCHAR(10) NOT NULL CHECK (role IN ('viewer','operator','admin')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, project_id)
);

-- Requests are audited with their project and HTTP status.
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS project_id UUID;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS status INTEGER;
CREATE INDEX IF NOT EXISTS idx_audit_project ON audit_logs(project_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_logs(recorded_at DESC);

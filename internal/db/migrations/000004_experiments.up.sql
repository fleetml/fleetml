-- Drop the old experiments table from migration 1 (different schema)
-- and experiment_arms which depends on it
DROP TABLE IF EXISTS experiment_arms CASCADE;
DROP TABLE IF EXISTS experiments CASCADE;

CREATE TABLE experiments (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id           UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name                 VARCHAR(255) NOT NULL,
    description          TEXT DEFAULT '',
    status               VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
                         CHECK (status IN ('DRAFT','RUNNING','PAUSED','CONCLUDED','CANCELLED')),
    control_version_id   UUID NOT NULL REFERENCES model_versions(id),
    treatment_version_id UUID NOT NULL REFERENCES model_versions(id),
    traffic_percent      INTEGER NOT NULL DEFAULT 100 CHECK (traffic_percent > 0 AND traffic_percent <= 100),
    split_ratio          INTEGER NOT NULL DEFAULT 50 CHECK (split_ratio > 0 AND split_ratio < 100),
    target_selector      JSONB DEFAULT '{}',
    primary_metric       VARCHAR(50) NOT NULL DEFAULT 'latency',
    min_sample_size      INTEGER NOT NULL DEFAULT 100,
    confidence_level     DOUBLE PRECISION NOT NULL DEFAULT 0.95,
    auto_conclude        BOOLEAN NOT NULL DEFAULT false,
    max_duration         VARCHAR(20) DEFAULT '7d',
    result               JSONB,
    winner_version_id    UUID REFERENCES model_versions(id),
    conclusion_note      TEXT DEFAULT '',
    created_by           VARCHAR(255) NOT NULL DEFAULT 'system',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at           TIMESTAMPTZ,
    concluded_at         TIMESTAMPTZ,
    UNIQUE(project_id, name)
);

CREATE INDEX idx_experiments_project_w9 ON experiments(project_id);
CREATE INDEX idx_experiments_status_w9 ON experiments(status) WHERE status IN ('RUNNING', 'DRAFT');

CREATE TABLE IF NOT EXISTS cohort_assignments (
    device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    experiment_id   UUID NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    cohort          VARCHAR(20) NOT NULL CHECK (cohort IN ('control', 'treatment')),
    assigned_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (device_id, experiment_id)
);

CREATE INDEX IF NOT EXISTS idx_cohort_experiment ON cohort_assignments(experiment_id);

CREATE TRIGGER set_updated_at BEFORE UPDATE ON experiments FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

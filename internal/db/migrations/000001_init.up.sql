-- Enable extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Projects
CREATE TABLE projects (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(255) NOT NULL,
    slug        VARCHAR(255) NOT NULL UNIQUE,
    org_id      UUID,
    description TEXT DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_projects_org ON projects(org_id);
CREATE INDEX idx_projects_slug ON projects(slug);

-- Models
CREATE TABLE models (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    framework   VARCHAR(50) NOT NULL CHECK (framework IN ('TFLITE','ONNX','COREML','GGUF','CUSTOM')),
    task_type   VARCHAR(50) NOT NULL CHECK (task_type IN ('CLASSIFICATION','DETECTION','SEGMENTATION','LLM','STT','TTS','VLM','RECOMMENDATION','ANOMALY','CUSTOM')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by  VARCHAR(255) NOT NULL DEFAULT 'system',
    UNIQUE(project_id, name)
);
CREATE INDEX idx_models_project ON models(project_id);

-- Model Versions
CREATE TABLE model_versions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    model_id        UUID NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    version         VARCHAR(50) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
                    CHECK (status IN ('DRAFT','STAGED','ACTIVE','ARCHIVED')),
    metadata_json   JSONB DEFAULT '{}',
    release_notes   TEXT DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    promoted_at     TIMESTAMPTZ,
    promoted_by     VARCHAR(255),
    UNIQUE(model_id, version)
);
CREATE INDEX idx_model_versions_model ON model_versions(model_id);
CREATE INDEX idx_model_versions_status ON model_versions(status);

-- Model Variants (quantization levels per version)
CREATE TABLE model_variants (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    version_id          UUID NOT NULL REFERENCES model_versions(id) ON DELETE CASCADE,
    quantization        VARCHAR(20) NOT NULL CHECK (quantization IN ('FP32','FP16','INT8','INT4','DYNAMIC')),
    target_runtime      VARCHAR(50) NOT NULL CHECK (target_runtime IN ('TFLITE','ONNX','COREML','GGUF','CUSTOM')),
    min_ram_mb          INT NOT NULL DEFAULT 0,
    min_npu_tflops      FLOAT,
    blob_key            VARCHAR(1024) NOT NULL,
    blob_size_bytes     BIGINT NOT NULL,
    blob_sha256         VARCHAR(64) NOT NULL,
    benchmark_latency_ms FLOAT,
    benchmark_accuracy   FLOAT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(version_id, quantization, target_runtime)
);
CREATE INDEX idx_model_variants_version ON model_variants(version_id);

-- Device Classes (templates for device categories)
CREATE TABLE device_classes (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            VARCHAR(255) NOT NULL UNIQUE,
    os              VARCHAR(20) NOT NULL CHECK (os IN ('ANDROID','IOS','LINUX','WINDOWS')),
    min_ram_mb      INT NOT NULL DEFAULT 0,
    has_npu         BOOLEAN NOT NULL DEFAULT FALSE,
    has_gpu         BOOLEAN NOT NULL DEFAULT FALSE,
    npu_tflops      FLOAT,
    chipset_family  VARCHAR(255),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Devices (registered edge devices)
CREATE TABLE devices (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_class_id UUID REFERENCES device_classes(id),
    fingerprint     VARCHAR(64) NOT NULL UNIQUE,
    os              VARCHAR(20) NOT NULL CHECK (os IN ('ANDROID','IOS','LINUX','WINDOWS')),
    os_version      VARCHAR(50) NOT NULL DEFAULT '',
    ram_mb          INT NOT NULL DEFAULT 0,
    chipset         VARCHAR(255) DEFAULT '',
    has_npu         BOOLEAN NOT NULL DEFAULT FALSE,
    has_gpu         BOOLEAN NOT NULL DEFAULT FALSE,
    npu_tflops      FLOAT,
    agent_version   VARCHAR(50) NOT NULL DEFAULT '',
    labels_json     JSONB DEFAULT '{}',
    auth_token_hash VARCHAR(64),
    last_seen_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_devices_fingerprint ON devices(fingerprint);
CREATE INDEX idx_devices_class ON devices(device_class_id);
CREATE INDEX idx_devices_labels ON devices USING GIN(labels_json);
CREATE INDEX idx_devices_last_seen ON devices(last_seen_at);

-- Deployments
CREATE TABLE deployments (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id          UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    model_version_id    UUID NOT NULL REFERENCES model_versions(id),
    name                VARCHAR(255) NOT NULL DEFAULT '',
    target_selector     JSONB NOT NULL DEFAULT '{}',
    strategy            VARCHAR(20) NOT NULL DEFAULT 'ALL_AT_ONCE'
                        CHECK (strategy IN ('ALL_AT_ONCE','CANARY','ROLLING')),
    canary_percentage   INT CHECK (canary_percentage >= 0 AND canary_percentage <= 100),
    status              VARCHAR(20) NOT NULL DEFAULT 'PENDING'
                        CHECK (status IN ('PENDING','ROLLING_OUT','COMPLETE','FAILED','CANCELLED')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,
    created_by          VARCHAR(255) NOT NULL DEFAULT 'system'
);
CREATE INDEX idx_deployments_project ON deployments(project_id);
CREATE INDEX idx_deployments_version ON deployments(model_version_id);
CREATE INDEX idx_deployments_status ON deployments(status);

-- Experiments (A/B tests)
CREATE TABLE experiments (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id          UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name                VARCHAR(255) NOT NULL,
    description         TEXT DEFAULT '',
    status              VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
                        CHECK (status IN ('DRAFT','RUNNING','CONCLUDED','CANCELLED')),
    target_selector     JSONB NOT NULL DEFAULT '{}',
    traffic_split_json  JSONB NOT NULL DEFAULT '{}',
    primary_metric      VARCHAR(100) NOT NULL DEFAULT 'latency_p50',
    min_sample_size     INT NOT NULL DEFAULT 1000,
    confidence_level    FLOAT NOT NULL DEFAULT 0.95,
    started_at          TIMESTAMPTZ,
    concluded_at        TIMESTAMPTZ,
    winner_arm_id       UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by          VARCHAR(255) NOT NULL DEFAULT 'system'
);
CREATE INDEX idx_experiments_project ON experiments(project_id);
CREATE INDEX idx_experiments_status ON experiments(status);

-- Experiment Arms
CREATE TABLE experiment_arms (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    experiment_id       UUID NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    name                VARCHAR(100) NOT NULL,
    model_version_id    UUID NOT NULL REFERENCES model_versions(id),
    traffic_percentage  INT NOT NULL CHECK (traffic_percentage >= 0 AND traffic_percentage <= 100),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(experiment_id, name)
);
CREATE INDEX idx_experiment_arms_experiment ON experiment_arms(experiment_id);

-- Telemetry Events (append-only, high volume — TimescaleDB hypertable)
CREATE TABLE telemetry_events (
    id                  BIGSERIAL,
    device_id           UUID NOT NULL REFERENCES devices(id),
    model_version_id    UUID NOT NULL REFERENCES model_versions(id),
    experiment_arm_id   UUID REFERENCES experiment_arms(id),
    event_type          VARCHAR(30) NOT NULL
                        CHECK (event_type IN ('INFERENCE','MODEL_LOAD','MODEL_LOAD_FAILED','OOM','PREDICTION_DRIFT','ERROR')),
    latency_ms          INT,
    confidence_score    FLOAT,
    prediction_label    VARCHAR(255),
    memory_usage_mb     INT,
    battery_impact_pct  FLOAT,
    error_message       TEXT,
    recorded_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, recorded_at)
);

-- Convert to TimescaleDB hypertable for time-series optimization
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        PERFORM create_hypertable('telemetry_events', 'recorded_at',
            chunk_time_interval => INTERVAL '7 days',
            if_not_exists => TRUE
        );
    END IF;
END $$;

CREATE INDEX idx_telemetry_device ON telemetry_events(device_id, recorded_at DESC);
CREATE INDEX idx_telemetry_model ON telemetry_events(model_version_id, recorded_at DESC);
CREATE INDEX idx_telemetry_experiment ON telemetry_events(experiment_arm_id, recorded_at DESC)
    WHERE experiment_arm_id IS NOT NULL;
CREATE INDEX idx_telemetry_type ON telemetry_events(event_type, recorded_at DESC);

-- Audit Log (append-only)
CREATE TABLE audit_logs (
    id              BIGSERIAL PRIMARY KEY,
    actor           VARCHAR(255) NOT NULL,
    action          VARCHAR(100) NOT NULL,
    resource_type   VARCHAR(50) NOT NULL,
    resource_id     UUID NOT NULL,
    details_json    JSONB DEFAULT '{}',
    ip_address      VARCHAR(45),
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_resource ON audit_logs(resource_type, resource_id);
CREATE INDEX idx_audit_actor ON audit_logs(actor, recorded_at DESC);
CREATE INDEX idx_audit_action ON audit_logs(action, recorded_at DESC);

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply updated_at triggers
CREATE TRIGGER set_updated_at BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON models FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON model_versions FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON devices FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON deployments FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
CREATE TRIGGER set_updated_at BEFORE UPDATE ON experiments FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

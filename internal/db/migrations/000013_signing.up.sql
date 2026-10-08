-- Per-project ed25519 artifact signing keys. Private keys are stored
-- encrypted (AES-256-GCM with a key derived from SIGNING_MASTER_KEY).
CREATE TABLE IF NOT EXISTS signing_keys (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id       UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    key_id           VARCHAR(32) NOT NULL UNIQUE,
    public_key       BYTEA NOT NULL,
    private_key_enc  BYTEA NOT NULL,
    status           VARCHAR(10) NOT NULL CHECK (status IN ('active','retiring')),
    not_after        TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_signing_keys_active ON signing_keys(project_id) WHERE status = 'active';

-- Each READY variant is signed when it is finalized.
ALTER TABLE model_variants ADD COLUMN IF NOT EXISTS signature BYTEA;
ALTER TABLE model_variants ADD COLUMN IF NOT EXISTS signing_key_id VARCHAR(32) NOT NULL DEFAULT '';

ALTER TABLE telemetry_events DROP CONSTRAINT IF EXISTS telemetry_events_event_type_check;
ALTER TABLE telemetry_events ADD CONSTRAINT telemetry_events_event_type_check
    CHECK (event_type IN ('INFERENCE','MODEL_LOAD','MODEL_LOAD_FAILED','OOM','PREDICTION_DRIFT','ERROR','SIGNATURE_INVALID'));

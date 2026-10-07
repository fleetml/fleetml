CREATE TABLE IF NOT EXISTS drift_results (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    model_version_id  UUID NOT NULL,
    metric            VARCHAR(50) NOT NULL,
    psi_value         DOUBLE PRECISION NOT NULL,
    severity          VARCHAR(20) NOT NULL,
    baseline_from     TIMESTAMPTZ NOT NULL,
    baseline_to       TIMESTAMPTZ NOT NULL,
    current_from      TIMESTAMPTZ NOT NULL,
    current_to        TIMESTAMPTZ NOT NULL,
    baseline_sample   INTEGER NOT NULL DEFAULT 0,
    current_sample    INTEGER NOT NULL DEFAULT 0,
    message           TEXT DEFAULT '',
    checked_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_drift_version_time ON drift_results(model_version_id, checked_at DESC);

-- Feature-level drift: the bin edges of each model's input features, the
-- histograms devices report with inferences, and every drift check's PSI.
CREATE TABLE IF NOT EXISTS model_features (
    model_id    UUID NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    name        VARCHAR(100) NOT NULL,
    edges       DOUBLE PRECISION[] NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (model_id, name)
);

CREATE TABLE IF NOT EXISTS feature_histograms (
    recorded_at       TIMESTAMPTZ NOT NULL,
    model_version_id  UUID NOT NULL,
    feature           VARCHAR(100) NOT NULL,
    counts            INTEGER[] NOT NULL
);
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        PERFORM create_hypertable('feature_histograms', 'recorded_at',
            chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_feature_histograms_version ON feature_histograms(model_version_id, feature, recorded_at DESC);

CREATE TABLE IF NOT EXISTS drift_points (
    model_version_id  UUID NOT NULL,
    metric            VARCHAR(110) NOT NULL,
    psi               DOUBLE PRECISION NOT NULL,
    severity          VARCHAR(20) NOT NULL,
    checked_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_drift_points_version ON drift_points(model_version_id, checked_at DESC);

-- "feature:" plus a feature name.
ALTER TABLE drift_results ALTER COLUMN metric TYPE VARCHAR(110);

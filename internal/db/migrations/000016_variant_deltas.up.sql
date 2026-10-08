-- Binary deltas between consecutive READY variants of a model (same runtime
-- and quantization), so a device that has the base downloads only the delta.
CREATE TABLE IF NOT EXISTS variant_deltas (
    base_variant_id    UUID NOT NULL REFERENCES model_variants(id) ON DELETE CASCADE,
    target_variant_id  UUID NOT NULL REFERENCES model_variants(id) ON DELETE CASCADE,
    blob_key           VARCHAR(1024) NOT NULL,
    size_bytes         BIGINT NOT NULL,
    sha256             VARCHAR(64) NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (base_variant_id, target_variant_id)
);
CREATE INDEX IF NOT EXISTS idx_variant_deltas_target ON variant_deltas(target_variant_id);

-- Variant lifecycle: UPLOADING until finalize verifies the blob's sha256,
-- then READY (assignable) or FAILED. Variants that existed before this
-- migration were already assignable, so they start as READY.
ALTER TABLE model_variants ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'READY';
ALTER TABLE model_variants ALTER COLUMN status SET DEFAULT 'UPLOADING';
ALTER TABLE model_variants ADD CONSTRAINT model_variants_status_check
    CHECK (status IN ('UPLOADING', 'READY', 'FAILED'));

ALTER TABLE model_variants DROP CONSTRAINT IF EXISTS model_variants_status_check;
ALTER TABLE model_variants DROP COLUMN IF EXISTS status;

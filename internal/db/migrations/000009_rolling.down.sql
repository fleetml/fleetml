ALTER TABLE devices DROP COLUMN IF EXISTS loaded_variant_ids;
ALTER TABLE deployments DROP COLUMN IF EXISTS rollout_threshold;
ALTER TABLE deployments DROP COLUMN IF EXISTS stage_started_at;

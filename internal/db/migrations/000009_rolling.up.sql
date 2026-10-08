-- Rolling deployments: when the current stage started (the health gate's
-- window and the batch interval clock), and the hash-ring threshold below
-- which devices are exposed.
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS stage_started_at TIMESTAMPTZ;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS rollout_threshold BIGINT;

-- Variants each device reported loaded in its last heartbeat; used to count
-- unavailable devices (max_unavailable) during a rolling deployment.
ALTER TABLE devices ADD COLUMN IF NOT EXISTS loaded_variant_ids JSONB NOT NULL DEFAULT '[]';

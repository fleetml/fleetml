-- Add deployment engine columns
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS canary_stages JSONB DEFAULT '[]';
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS current_stage INTEGER DEFAULT 0;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS rolling_config JSONB;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS health_gate JSONB;
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS previous_version_id UUID REFERENCES model_versions(id);
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS rollback_reason TEXT DEFAULT '';

-- Allow new status values
ALTER TABLE deployments DROP CONSTRAINT IF EXISTS deployments_status_check;
ALTER TABLE deployments ADD CONSTRAINT deployments_status_check
    CHECK (status IN ('PENDING','ROLLING_OUT','PAUSED','COMPLETE','ROLLED_BACK','FAILED','CANCELLED'));

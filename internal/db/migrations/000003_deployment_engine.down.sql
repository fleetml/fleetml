ALTER TABLE deployments DROP CONSTRAINT IF EXISTS deployments_status_check;
ALTER TABLE deployments ADD CONSTRAINT deployments_status_check
    CHECK (status IN ('PENDING','ROLLING_OUT','COMPLETE','FAILED','CANCELLED'));

ALTER TABLE deployments DROP COLUMN IF EXISTS rollback_reason;
ALTER TABLE deployments DROP COLUMN IF EXISTS previous_version_id;
ALTER TABLE deployments DROP COLUMN IF EXISTS health_gate;
ALTER TABLE deployments DROP COLUMN IF EXISTS rolling_config;
ALTER TABLE deployments DROP COLUMN IF EXISTS current_stage;
ALTER TABLE deployments DROP COLUMN IF EXISTS canary_stages;

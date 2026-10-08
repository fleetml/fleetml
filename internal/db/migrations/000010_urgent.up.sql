-- Urgent deployments download on any network, ignoring the device download policy.
ALTER TABLE deployments ADD COLUMN IF NOT EXISTS urgent BOOLEAN NOT NULL DEFAULT false;

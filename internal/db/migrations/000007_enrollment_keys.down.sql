DROP INDEX IF EXISTS idx_devices_project;
ALTER TABLE devices DROP COLUMN IF EXISTS project_id;
DROP TABLE IF EXISTS enrollment_keys;

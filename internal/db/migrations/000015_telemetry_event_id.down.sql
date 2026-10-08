DROP INDEX IF EXISTS idx_telemetry_event_id;
ALTER TABLE telemetry_events DROP COLUMN IF EXISTS event_id;

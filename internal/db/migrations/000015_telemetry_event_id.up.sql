-- Telemetry is stored once per event ID, so a redelivered or resent batch is
-- not counted twice. (A unique index on a hypertable includes its time column.)
ALTER TABLE telemetry_events ADD COLUMN IF NOT EXISTS event_id UUID;
CREATE UNIQUE INDEX IF NOT EXISTS idx_telemetry_event_id ON telemetry_events(event_id, recorded_at);

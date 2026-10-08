ALTER TABLE telemetry_events DROP CONSTRAINT IF EXISTS telemetry_events_device_id_fkey;
ALTER TABLE telemetry_events ADD CONSTRAINT telemetry_events_device_id_fkey
    FOREIGN KEY (device_id) REFERENCES devices(id);

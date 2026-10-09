DROP TRIGGER IF EXISTS keep_experiment_arms ON devices;
DROP FUNCTION IF EXISTS telemetry_keep_experiment_arms();
DELETE FROM telemetry_events WHERE device_id IS NULL;
ALTER TABLE telemetry_events DROP CONSTRAINT IF EXISTS telemetry_events_device_id_fkey;
ALTER TABLE telemetry_events ADD CONSTRAINT telemetry_events_device_id_fkey
    FOREIGN KEY (device_id) REFERENCES devices(id);
ALTER TABLE telemetry_events ALTER COLUMN device_id SET NOT NULL;

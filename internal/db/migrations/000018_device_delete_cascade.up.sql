-- Deleting a device deletes its telemetry, as it already deleted its cohort
-- assignments. Before, a device that had reported any telemetry could not be
-- deleted (the foreign key had no ON DELETE action).
ALTER TABLE telemetry_events DROP CONSTRAINT IF EXISTS telemetry_events_device_id_fkey;
ALTER TABLE telemetry_events ADD CONSTRAINT telemetry_events_device_id_fkey
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE;

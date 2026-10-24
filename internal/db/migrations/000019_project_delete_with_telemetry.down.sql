ALTER TABLE telemetry_events DROP CONSTRAINT IF EXISTS telemetry_events_model_version_id_fkey;
ALTER TABLE telemetry_events ADD CONSTRAINT telemetry_events_model_version_id_fkey
    FOREIGN KEY (model_version_id) REFERENCES model_versions(id);

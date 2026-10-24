-- Deleting a model version (with its model or project) deletes its
-- telemetry. Before, a version that had any telemetry could not be deleted:
-- the foreign key had no ON DELETE action, so `fleetml project delete` failed
-- for every project whose devices had reported.
ALTER TABLE telemetry_events DROP CONSTRAINT IF EXISTS telemetry_events_model_version_id_fkey;
ALTER TABLE telemetry_events ADD CONSTRAINT telemetry_events_model_version_id_fkey
    FOREIGN KEY (model_version_id) REFERENCES model_versions(id) ON DELETE CASCADE;

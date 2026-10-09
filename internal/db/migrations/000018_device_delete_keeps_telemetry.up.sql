-- Deleting a device keeps its telemetry: the rows stay, with their model
-- version and experiment arm, and lose only the link to the device. Before, a
-- device that had reported any telemetry could not be deleted (the foreign key
-- had no ON DELETE action).
ALTER TABLE telemetry_events ALTER COLUMN device_id DROP NOT NULL;
ALTER TABLE telemetry_events DROP CONSTRAINT IF EXISTS telemetry_events_device_id_fkey;
ALTER TABLE telemetry_events ADD CONSTRAINT telemetry_events_device_id_fkey
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE SET NULL;

-- Experiment results count an event without an arm ID by its device's cohort
-- assignment, which is deleted with the device. Before that, give those events
-- the arm ID they were counted under (experiment.ArmID: a v5 UUID of the
-- experiment ID and the cohort), so the results do not change.
CREATE OR REPLACE FUNCTION telemetry_keep_experiment_arms() RETURNS trigger AS $$
BEGIN
    UPDATE telemetry_events te
       SET experiment_arm_id = uuid_generate_v5(ca.experiment_id, ca.cohort)
      FROM cohort_assignments ca
      JOIN experiments e ON e.id = ca.experiment_id
     WHERE ca.device_id = OLD.id
       AND te.device_id = OLD.id
       AND te.experiment_arm_id IS NULL
       AND te.model_version_id = CASE ca.cohort WHEN 'control' THEN e.control_version_id ELSE e.treatment_version_id END
       AND te.recorded_at >= COALESCE(e.started_at, e.created_at);
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER keep_experiment_arms BEFORE DELETE ON devices
    FOR EACH ROW EXECUTE FUNCTION telemetry_keep_experiment_arms();

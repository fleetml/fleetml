DELETE FROM telemetry_events WHERE event_type = 'SIGNATURE_INVALID';
ALTER TABLE telemetry_events DROP CONSTRAINT IF EXISTS telemetry_events_event_type_check;
ALTER TABLE telemetry_events ADD CONSTRAINT telemetry_events_event_type_check
    CHECK (event_type IN ('INFERENCE','MODEL_LOAD','MODEL_LOAD_FAILED','OOM','PREDICTION_DRIFT','ERROR'));
ALTER TABLE model_variants DROP COLUMN IF EXISTS signing_key_id;
ALTER TABLE model_variants DROP COLUMN IF EXISTS signature;
DROP TABLE IF EXISTS signing_keys;

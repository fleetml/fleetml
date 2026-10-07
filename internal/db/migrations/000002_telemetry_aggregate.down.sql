DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        PERFORM remove_retention_policy('telemetry_events', if_exists => TRUE);
    END IF;
END $$;

DROP MATERIALIZED VIEW IF EXISTS telemetry_hourly_stats;

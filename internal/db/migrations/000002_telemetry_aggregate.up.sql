-- Continuous aggregate: hourly telemetry stats per model_version
-- Uses the existing telemetry_events hypertable from migration 000001
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        EXECUTE '
        CREATE MATERIALIZED VIEW IF NOT EXISTS telemetry_hourly_stats
        WITH (timescaledb.continuous) AS
        SELECT
            time_bucket(''1 hour'', recorded_at) AS bucket,
            model_version_id,
            event_type,
            COUNT(*) AS total_events,
            COUNT(*) FILTER (WHERE error_message IS NOT NULL AND error_message != '''') AS error_count,
            AVG(latency_ms) AS avg_latency,
            percentile_cont(0.50) WITHIN GROUP (ORDER BY latency_ms) AS p50_latency,
            percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) AS p95_latency,
            percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms) AS p99_latency,
            MIN(latency_ms) AS min_latency,
            MAX(latency_ms) AS max_latency,
            AVG(confidence_score) AS avg_confidence,
            AVG(memory_usage_mb) AS avg_memory_mb
        FROM telemetry_events
        GROUP BY bucket, model_version_id, event_type
        WITH NO DATA';

        PERFORM add_continuous_aggregate_policy('telemetry_hourly_stats',
            start_offset => INTERVAL '15 days',
            end_offset => INTERVAL '1 hour',
            schedule_interval => INTERVAL '1 hour',
            if_not_exists => TRUE
        );
    END IF;
END $$;

-- Retention: keep raw events for 30 days
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb') THEN
        PERFORM add_retention_policy('telemetry_events', INTERVAL '30 days', if_not_exists => TRUE);
    END IF;
END $$;

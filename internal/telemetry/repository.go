package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides data access for telemetry events.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new telemetry repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// InsertEvents batch-inserts telemetry events using CopyFrom for high throughput.
func (r *Repository) InsertEvents(ctx context.Context, events []Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}

	rows := make([][]interface{}, len(events))
	for i, e := range events {
		rows[i] = []interface{}{
			e.DeviceID, e.ModelVersionID, e.ExperimentArmID, e.EventType,
			e.LatencyMs, e.Confidence, e.PredictionLabel,
			e.MemoryUsageMB, e.BatteryPercent, e.ErrorMessage, e.RecordedAt,
		}
	}

	count, err := r.pool.CopyFrom(ctx,
		pgx.Identifier{"telemetry_events"},
		[]string{"device_id", "model_version_id", "experiment_arm_id", "event_type",
			"latency_ms", "confidence_score", "prediction_label",
			"memory_usage_mb", "battery_impact_pct", "error_message", "recorded_at"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, fmt.Errorf("telemetry.InsertEvents: %w", err)
	}
	return int(count), nil
}

// InsertEvent inserts a single telemetry event. Used as a fallback when a
// batch CopyFrom fails, so that one bad row does not block the others.
func (r *Repository) InsertEvent(ctx context.Context, e Event) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO telemetry_events (device_id, model_version_id, experiment_arm_id, event_type,
			latency_ms, confidence_score, prediction_label,
			memory_usage_mb, battery_impact_pct, error_message, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		e.DeviceID, e.ModelVersionID, e.ExperimentArmID, e.EventType,
		e.LatencyMs, e.Confidence, e.PredictionLabel,
		e.MemoryUsageMB, e.BatteryPercent, e.ErrorMessage, e.RecordedAt,
	)
	if err != nil {
		return fmt.Errorf("telemetry.InsertEvent: %w", err)
	}
	return nil
}

// KnownModelVersions returns the subset of ids that exist in model_versions.
func (r *Repository) KnownModelVersions(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	known := make(map[uuid.UUID]bool, len(ids))
	if len(ids) == 0 {
		return known, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT id FROM model_versions WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("telemetry.KnownModelVersions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("telemetry.KnownModelVersions: scanning: %w", err)
		}
		known[id] = true
	}
	return known, rows.Err()
}

// QueryEvents returns filtered telemetry events.
func (r *Repository) QueryEvents(ctx context.Context, filter EventFilter) ([]Event, int, error) {
	query := `SELECT device_id, model_version_id, event_type, COALESCE(latency_ms,0),
		COALESCE(confidence_score,0), COALESCE(prediction_label,''), COALESCE(memory_usage_mb,0),
		COALESCE(battery_impact_pct,0), COALESCE(error_message,''), recorded_at,
		COUNT(*) OVER() AS total
		FROM telemetry_events WHERE recorded_at >= $1 AND recorded_at <= $2`

	args := []interface{}{filter.From, filter.To}
	argIdx := 3

	if filter.DeviceID != nil {
		query += fmt.Sprintf(" AND device_id = $%d", argIdx)
		args = append(args, *filter.DeviceID)
		argIdx++
	}
	if filter.ModelVersionID != nil {
		query += fmt.Sprintf(" AND model_version_id = $%d", argIdx)
		args = append(args, *filter.ModelVersionID)
		argIdx++
	}
	if filter.EventType != "" {
		query += fmt.Sprintf(" AND event_type = $%d", argIdx)
		args = append(args, filter.EventType)
		argIdx++
	}
	if filter.ErrorOnly {
		query += " AND error_message IS NOT NULL AND error_message != ''"
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query += fmt.Sprintf(" ORDER BY recorded_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, filter.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("telemetry.QueryEvents: %w", err)
	}
	defer rows.Close()

	var events []Event
	var total int
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.DeviceID, &e.ModelVersionID, &e.EventType,
			&e.LatencyMs, &e.Confidence, &e.PredictionLabel,
			&e.MemoryUsageMB, &e.BatteryPercent, &e.ErrorMessage, &e.RecordedAt,
			&total); err != nil {
			return nil, 0, fmt.Errorf("telemetry.QueryEvents: scanning: %w", err)
		}
		events = append(events, e)
	}
	return events, total, rows.Err()
}

// GetSummary returns aggregated stats for a model version over a time range.
func (r *Repository) GetSummary(ctx context.Context, modelVersionID uuid.UUID, from, to time.Time) (*Summary, error) {
	query := `SELECT
		COUNT(*) AS total_events,
		COUNT(*) FILTER (WHERE error_message IS NOT NULL AND error_message != '') AS error_count,
		COALESCE(AVG(latency_ms), 0) AS avg_latency,
		COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY latency_ms), 0) AS p50,
		COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms), 0) AS p95,
		COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms), 0) AS p99,
		COALESCE(MIN(latency_ms), 0) AS min_lat,
		COALESCE(MAX(latency_ms), 0) AS max_lat,
		COALESCE(AVG(confidence_score), 0) AS avg_conf,
		COALESCE(AVG(memory_usage_mb), 0) AS avg_mem
		FROM telemetry_events
		WHERE model_version_id = $1 AND recorded_at >= $2 AND recorded_at <= $3`

	s := &Summary{ModelVersionID: modelVersionID, From: from, To: to}
	err := r.pool.QueryRow(ctx, query, modelVersionID, from, to).Scan(
		&s.TotalEvents, &s.ErrorCount, &s.AvgLatency,
		&s.P50Latency, &s.P95Latency, &s.P99Latency,
		&s.MinLatency, &s.MaxLatency, &s.AvgConfidence, &s.AvgMemoryMB,
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry.GetSummary: %w", err)
	}
	return s, nil
}

// GetSummaryByDeviceClass returns aggregated stats broken down by device class.
func (r *Repository) GetSummaryByDeviceClass(ctx context.Context, modelVersionID uuid.UUID, from, to time.Time) ([]Summary, error) {
	query := `SELECT
		COALESCE(dc.name, 'unknown') AS device_class,
		COUNT(*) AS total_events,
		COUNT(*) FILTER (WHERE te.error_message IS NOT NULL AND te.error_message != '') AS error_count,
		COALESCE(AVG(te.latency_ms), 0) AS avg_latency,
		COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY te.latency_ms), 0) AS p50,
		COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY te.latency_ms), 0) AS p95,
		COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY te.latency_ms), 0) AS p99,
		COALESCE(MIN(te.latency_ms), 0) AS min_lat,
		COALESCE(MAX(te.latency_ms), 0) AS max_lat,
		COALESCE(AVG(te.confidence_score), 0) AS avg_conf,
		COALESCE(AVG(te.memory_usage_mb), 0) AS avg_mem
		FROM telemetry_events te
		JOIN devices d ON d.id = te.device_id
		LEFT JOIN device_classes dc ON dc.id = d.device_class_id
		WHERE te.model_version_id = $1 AND te.recorded_at >= $2 AND te.recorded_at <= $3
		GROUP BY dc.name
		ORDER BY total_events DESC`

	rows, err := r.pool.Query(ctx, query, modelVersionID, from, to)
	if err != nil {
		return nil, fmt.Errorf("telemetry.GetSummaryByDeviceClass: %w", err)
	}
	defer rows.Close()

	var summaries []Summary
	for rows.Next() {
		s := Summary{ModelVersionID: modelVersionID, From: from, To: to}
		if err := rows.Scan(
			&s.DeviceClass,
			&s.TotalEvents, &s.ErrorCount, &s.AvgLatency,
			&s.P50Latency, &s.P95Latency, &s.P99Latency,
			&s.MinLatency, &s.MaxLatency, &s.AvgConfidence, &s.AvgMemoryMB,
		); err != nil {
			return nil, fmt.Errorf("telemetry.GetSummaryByDeviceClass: scanning: %w", err)
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// GetTimeSeries returns time-bucketed latency data for charting.
func (r *Repository) GetTimeSeries(ctx context.Context, modelVersionID uuid.UUID, from, to time.Time, interval string) ([]TimeSeriesPoint, error) {
	validIntervals := map[string]bool{"1m": true, "5m": true, "15m": true, "1h": true, "1d": true}
	if !validIntervals[interval] {
		interval = "1h"
	}

	// Map shorthand to PostgreSQL interval
	pgInterval := map[string]string{
		"1m": "1 minute", "5m": "5 minutes", "15m": "15 minutes", "1h": "1 hour", "1d": "1 day",
	}[interval]

	query := fmt.Sprintf(`SELECT
		time_bucket('%s', recorded_at) AS bucket,
		COALESCE(AVG(latency_ms), 0),
		COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms), 0),
		COUNT(*),
		COUNT(*) FILTER (WHERE error_message IS NOT NULL AND error_message != '')
		FROM telemetry_events
		WHERE model_version_id = $1 AND recorded_at >= $2 AND recorded_at <= $3
		GROUP BY bucket ORDER BY bucket`, pgInterval)

	rows, err := r.pool.Query(ctx, query, modelVersionID, from, to)
	if err != nil {
		return nil, fmt.Errorf("telemetry.GetTimeSeries: %w", err)
	}
	defer rows.Close()

	var points []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		if err := rows.Scan(&p.Time, &p.AvgLatency, &p.P95Latency, &p.Count, &p.ErrorCount); err != nil {
			return nil, fmt.Errorf("telemetry.GetTimeSeries: scanning: %w", err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

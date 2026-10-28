package deploy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TelemetryAdapter implements TelemetryQuerier using pgxpool.
type TelemetryAdapter struct {
	pool *pgxpool.Pool
}

// NewTelemetryAdapter creates a new TelemetryAdapter.
func NewTelemetryAdapter(pool *pgxpool.Pool) *TelemetryAdapter {
	return &TelemetryAdapter{pool: pool}
}

// errorEvent matches events that count as failures for the health gate.
const errorEvent = `(event_type IN ('ERROR', 'OOM', 'MODEL_LOAD_FAILED', 'SIGNATURE_INVALID') OR (error_message IS NOT NULL AND error_message <> ''))`

func (a *TelemetryAdapter) CohortStats(ctx context.Context, modelVersionID uuid.UUID, deviceIDs []uuid.UUID, since time.Time) (*DeploymentStats, error) {
	stats := &DeploymentStats{}
	if len(deviceIDs) == 0 {
		return stats, nil
	}
	err := a.pool.QueryRow(ctx, `SELECT
		COUNT(*),
		COUNT(*) FILTER (WHERE `+errorEvent+`),
		COALESCE(AVG(latency_ms) FILTER (WHERE event_type = 'INFERENCE'), 0),
		COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE event_type = 'INFERENCE' AND latency_ms > 0), 0)
		FROM telemetry_events
		WHERE model_version_id = $1 AND recorded_at >= $2 AND device_id = ANY($3)`,
		modelVersionID, since, deviceIDs,
	).Scan(&stats.TotalEvents, &stats.ErrorCount, &stats.AvgLatencyMs, &stats.P95LatencyMs)
	if err != nil {
		return nil, fmt.Errorf("telemetryAdapter.CohortStats: %w", err)
	}
	if stats.TotalEvents > 0 {
		stats.ErrorRate = float64(stats.ErrorCount) / float64(stats.TotalEvents)
	}
	return stats, nil
}

func (a *TelemetryAdapter) VersionP95(ctx context.Context, modelVersionID uuid.UUID, since time.Time) (p95 float64, events int64, err error) {
	err = a.pool.QueryRow(ctx, `SELECT
		COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms), 0), COUNT(*)
		FROM telemetry_events
		WHERE model_version_id = $1 AND recorded_at >= $2 AND event_type = 'INFERENCE' AND latency_ms > 0`,
		modelVersionID, since,
	).Scan(&p95, &events)
	if err != nil {
		return 0, 0, fmt.Errorf("telemetryAdapter.VersionP95: %w", err)
	}
	return p95, events, nil
}

// VersionAdapter implements VersionChecker using pgxpool.
type VersionAdapter struct {
	pool *pgxpool.Pool
}

// NewVersionAdapter creates a new VersionAdapter.
func NewVersionAdapter(pool *pgxpool.Pool) *VersionAdapter {
	return &VersionAdapter{pool: pool}
}

func (a *VersionAdapter) ReadyVariantCount(ctx context.Context, versionID uuid.UUID) (int, error) {
	var n int
	err := a.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM model_variants WHERE version_id = $1 AND status = 'READY'`, versionID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("versionAdapter.ReadyVariantCount: %w", err)
	}
	return n, nil
}

// VersionStatus is the version's registry status, "" if there is no such version.
func (a *VersionAdapter) VersionStatus(ctx context.Context, versionID uuid.UUID) (string, error) {
	var status string
	err := a.pool.QueryRow(ctx, `SELECT status FROM model_versions WHERE id = $1`, versionID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("versionAdapter.VersionStatus: %w", err)
	}
	return status, nil
}

func (a *VersionAdapter) ProjectExists(ctx context.Context, projectID uuid.UUID) (bool, error) {
	var ok bool
	if err := a.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1)`, projectID).Scan(&ok); err != nil {
		return false, fmt.Errorf("versionAdapter.ProjectExists: %w", err)
	}
	return ok, nil
}

func (a *VersionAdapter) VersionProject(ctx context.Context, versionID uuid.UUID) (uuid.UUID, error) {
	var project uuid.UUID
	err := a.pool.QueryRow(ctx,
		`SELECT m.project_id FROM model_versions v JOIN models m ON m.id = v.model_id WHERE v.id = $1`, versionID).Scan(&project)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("versionAdapter.VersionProject: %w", err)
	}
	return project, nil
}

func (a *VersionAdapter) CurrentVersion(ctx context.Context, projectID, versionID uuid.UUID) (*uuid.UUID, error) {
	var current uuid.UUID
	err := a.pool.QueryRow(ctx, `
		SELECT d.model_version_id
		FROM deployments d
		JOIN model_versions dv ON dv.id = d.model_version_id
		WHERE d.project_id = $1 AND d.status = 'COMPLETE' AND d.model_version_id <> $2
		  AND dv.model_id = (SELECT model_id FROM model_versions WHERE id = $2)
		ORDER BY d.created_at DESC
		LIMIT 1`, projectID, versionID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("versionAdapter.CurrentVersion: %w", err)
	}
	return &current, nil
}

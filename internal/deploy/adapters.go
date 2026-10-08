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

func (a *TelemetryAdapter) GetRecentStats(ctx context.Context, modelVersionID uuid.UUID, since time.Time) (*DeploymentStats, error) {
	query := `SELECT
		COUNT(*) AS total,
		COUNT(*) FILTER (WHERE error_message IS NOT NULL AND error_message != '') AS errors,
		COALESCE(AVG(latency_ms), 0) AS avg_latency
		FROM telemetry_events
		WHERE model_version_id = $1 AND recorded_at >= $2`

	stats := &DeploymentStats{}
	err := a.pool.QueryRow(ctx, query, modelVersionID, since).Scan(
		&stats.TotalEvents, &stats.ErrorCount, &stats.AvgLatencyMs,
	)
	if err != nil {
		return nil, fmt.Errorf("telemetryAdapter.GetRecentStats: %w", err)
	}
	if stats.TotalEvents > 0 {
		stats.ErrorRate = float64(stats.ErrorCount) / float64(stats.TotalEvents)
	}
	return stats, nil
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

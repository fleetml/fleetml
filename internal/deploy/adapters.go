package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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

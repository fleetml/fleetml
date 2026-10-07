package drift

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Adapter implements TelemetryRepository, ResultStore, and ModelVersionQuerier.
type Adapter struct {
	pool *pgxpool.Pool
}

// NewAdapter creates a new drift adapter.
func NewAdapter(pool *pgxpool.Pool) *Adapter {
	return &Adapter{pool: pool}
}

func (a *Adapter) GetLatencyValues(ctx context.Context, modelVersionID uuid.UUID, from, to time.Time, limit int) ([]float64, error) {
	rows, err := a.pool.Query(ctx,
		`SELECT latency_ms FROM telemetry_events WHERE model_version_id=$1 AND recorded_at>=$2 AND recorded_at<=$3 AND latency_ms>0 ORDER BY recorded_at DESC LIMIT $4`,
		modelVersionID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("drift.GetLatencyValues: %w", err)
	}
	defer rows.Close()
	var values []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

func (a *Adapter) GetConfidenceValues(ctx context.Context, modelVersionID uuid.UUID, from, to time.Time, limit int) ([]float64, error) {
	rows, err := a.pool.Query(ctx,
		`SELECT confidence_score FROM telemetry_events WHERE model_version_id=$1 AND recorded_at>=$2 AND recorded_at<=$3 AND confidence_score>0 ORDER BY recorded_at DESC LIMIT $4`,
		modelVersionID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("drift.GetConfidenceValues: %w", err)
	}
	defer rows.Close()
	var values []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

func (a *Adapter) SaveDriftResult(ctx context.Context, r *DriftResult) error {
	_, err := a.pool.Exec(ctx,
		`INSERT INTO drift_results (model_version_id,metric,psi_value,severity,baseline_from,baseline_to,current_from,current_to,baseline_sample,current_sample,message,checked_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		r.ModelVersionID, r.Metric, r.PSIValue, r.Severity,
		r.BaselineFrom, r.BaselineTo, r.CurrentFrom, r.CurrentTo,
		r.BaselineSample, r.CurrentSample, r.Message, r.CheckedAt)
	return err
}

func (a *Adapter) GetLatestResults(ctx context.Context, modelVersionID uuid.UUID) ([]DriftResult, error) {
	rows, err := a.pool.Query(ctx,
		`SELECT DISTINCT ON (metric) id,model_version_id,metric,psi_value,severity,baseline_from,baseline_to,current_from,current_to,baseline_sample,current_sample,message,checked_at
		FROM drift_results WHERE model_version_id=$1 ORDER BY metric, checked_at DESC`, modelVersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDriftResults(rows)
}

func (a *Adapter) GetHistory(ctx context.Context, modelVersionID uuid.UUID, limit int) ([]DriftResult, error) {
	rows, err := a.pool.Query(ctx,
		`SELECT id,model_version_id,metric,psi_value,severity,baseline_from,baseline_to,current_from,current_to,baseline_sample,current_sample,message,checked_at
		FROM drift_results WHERE model_version_id=$1 ORDER BY checked_at DESC LIMIT $2`, modelVersionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDriftResults(rows)
}

func scanDriftResults(rows interface {
	Next() bool
	Scan(...interface{}) error
	Err() error
}) ([]DriftResult, error) {
	var results []DriftResult
	for rows.Next() {
		var r DriftResult
		if err := rows.Scan(&r.ID, &r.ModelVersionID, &r.Metric, &r.PSIValue, &r.Severity,
			&r.BaselineFrom, &r.BaselineTo, &r.CurrentFrom, &r.CurrentTo,
			&r.BaselineSample, &r.CurrentSample, &r.Message, &r.CheckedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ListActiveVersionIDs returns IDs of ACTIVE model versions.
func (a *Adapter) ListActiveVersionIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := a.pool.Query(ctx, `SELECT id FROM model_versions WHERE status='ACTIVE'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

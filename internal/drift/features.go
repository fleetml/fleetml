package drift

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// FeatureMetricPrefix starts the metric name of a feature's drift.
const FeatureMetricPrefix = "feature:"

// ErrModelNotFound is returned when configuring features of an unknown model.
var ErrModelNotFound = errors.New("model not found")

// FeatureSpec is the bin edges of one input feature of a model.
type FeatureSpec struct {
	Name  string    `json:"name"`
	Edges []float64 `json:"edges"`
}

// DeviceFeature is a feature a device should report for one of its models.
type DeviceFeature struct {
	ModelID string
	FeatureSpec
}

// Point is one drift check's PSI for a metric.
type Point struct {
	Metric    string    `json:"metric"`
	PSI       float64   `json:"psi"`
	Severity  string    `json:"severity"`
	CheckedAt time.Time `json:"checked_at"`
}

// ModelFeatures returns a model's feature specs, by name.
func (a *Adapter) ModelFeatures(ctx context.Context, modelID uuid.UUID) ([]FeatureSpec, error) {
	rows, err := a.pool.Query(ctx, `SELECT name, edges FROM model_features WHERE model_id = $1 ORDER BY name`, modelID)
	if err != nil {
		return nil, fmt.Errorf("drift.ModelFeatures: %w", err)
	}
	defer rows.Close()
	out := []FeatureSpec{}
	for rows.Next() {
		var f FeatureSpec
		if err := rows.Scan(&f.Name, &f.Edges); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SetModelFeatures replaces a model's feature specs.
func (a *Adapter) SetModelFeatures(ctx context.Context, modelID uuid.UUID, specs []FeatureSpec) error {
	tx, err := a.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM model_features WHERE model_id = $1`, modelID); err != nil {
		return fmt.Errorf("drift.SetModelFeatures: %w", err)
	}
	for _, f := range specs {
		if _, err := tx.Exec(ctx, `INSERT INTO model_features (model_id, name, edges) VALUES ($1, $2, $3)`, modelID, f.Name, f.Edges); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return ErrModelNotFound
			}
			return fmt.Errorf("drift.SetModelFeatures: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// FeatureSpecsForVersion returns the feature specs of a version's model.
func (a *Adapter) FeatureSpecsForVersion(ctx context.Context, versionID uuid.UUID) ([]FeatureSpec, error) {
	var modelID uuid.UUID
	if err := a.pool.QueryRow(ctx, `SELECT model_id FROM model_versions WHERE id = $1`, versionID).Scan(&modelID); err != nil {
		return nil, fmt.Errorf("drift.FeatureSpecsForVersion: %w", err)
	}
	return a.ModelFeatures(ctx, modelID)
}

// FeatureCounts sums a version's histograms of one feature (with the given
// number of bins) reported in [from, to).
func (a *Adapter) FeatureCounts(ctx context.Context, versionID uuid.UUID, feature string, bins int, from, to time.Time) ([]float64, error) {
	rows, err := a.pool.Query(ctx,
		`SELECT u.ord, SUM(u.c) FROM feature_histograms h, unnest(h.counts) WITH ORDINALITY AS u(c, ord)
		 WHERE h.model_version_id = $1 AND h.feature = $2 AND h.recorded_at >= $3 AND h.recorded_at < $4 AND cardinality(h.counts) = $5
		 GROUP BY u.ord`, versionID, feature, from, to, bins)
	if err != nil {
		return nil, fmt.Errorf("drift.FeatureCounts: %w", err)
	}
	defer rows.Close()
	counts := make([]float64, bins)
	for rows.Next() {
		var ord int
		var sum float64
		if err := rows.Scan(&ord, &sum); err != nil {
			return nil, err
		}
		counts[ord-1] = sum
	}
	return counts, rows.Err()
}

// DeviceFeatureBins returns the feature specs of every model in the device's project.
func (a *Adapter) DeviceFeatureBins(ctx context.Context, deviceID string) ([]DeviceFeature, error) {
	rows, err := a.pool.Query(ctx,
		`SELECT f.model_id::text, f.name, f.edges FROM model_features f JOIN models m ON m.id = f.model_id
		 WHERE m.project_id = (SELECT project_id FROM devices WHERE id = $1) ORDER BY f.model_id, f.name`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("drift.DeviceFeatureBins: %w", err)
	}
	defer rows.Close()
	var out []DeviceFeature
	for rows.Next() {
		var f DeviceFeature
		if err := rows.Scan(&f.ModelID, &f.Name, &f.Edges); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// AddPoint records one check's PSI, for the PSI-over-time chart.
func (a *Adapter) AddPoint(ctx context.Context, r *DriftResult) error {
	_, err := a.pool.Exec(ctx, `INSERT INTO drift_points (model_version_id, metric, psi, severity, checked_at) VALUES ($1, $2, $3, $4, $5)`,
		r.ModelVersionID, r.Metric, r.PSIValue, r.Severity, r.CheckedAt)
	if err != nil {
		return fmt.Errorf("drift.AddPoint: %w", err)
	}
	return nil
}

// Points returns a version's PSI points since a time, oldest first.
func (a *Adapter) Points(ctx context.Context, versionID uuid.UUID, since time.Time) ([]Point, error) {
	rows, err := a.pool.Query(ctx,
		`SELECT metric, psi, severity, checked_at FROM drift_points WHERE model_version_id = $1 AND checked_at >= $2 ORDER BY checked_at LIMIT 10000`,
		versionID, since)
	if err != nil {
		return nil, fmt.Errorf("drift.Points: %w", err)
	}
	defer rows.Close()
	out := []Point{}
	for rows.Next() {
		var p Point
		if err := rows.Scan(&p.Metric, &p.PSI, &p.Severity, &p.CheckedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

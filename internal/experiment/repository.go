package experiment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides data access for experiments.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new experiment repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const expColumns = `id, project_id, name, description, status,
	control_version_id, treatment_version_id, traffic_percent, split_ratio,
	target_selector, primary_metric, min_sample_size, confidence_level,
	auto_conclude, max_duration, result, winner_version_id, conclusion_note,
	created_by, created_at, updated_at, started_at, concluded_at`

func scanExperiment(row pgx.Row) (*Experiment, error) {
	e := &Experiment{}
	var selectorJSON, resultJSON []byte
	err := row.Scan(
		&e.ID, &e.ProjectID, &e.Name, &e.Description, &e.Status,
		&e.ControlVersionID, &e.TreatmentVersionID, &e.TrafficPercent, &e.SplitRatio,
		&selectorJSON, &e.PrimaryMetric, &e.MinSampleSize, &e.ConfidenceLevel,
		&e.AutoConclude, &e.MaxDuration, &resultJSON, &e.WinnerVersionID, &e.ConclusionNote,
		&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &e.StartedAt, &e.ConcludedAt,
	)
	if err != nil {
		return nil, err
	}
	if selectorJSON != nil {
		_ = json.Unmarshal(selectorJSON, &e.TargetSelector)
	}
	if resultJSON != nil {
		e.Result = &ExperimentResult{}
		_ = json.Unmarshal(resultJSON, e.Result)
	}
	return e, nil
}

func (r *Repository) Create(ctx context.Context, e *Experiment) error {
	selectorJSON, _ := json.Marshal(e.TargetSelector)
	var resultJSON []byte
	if e.Result != nil {
		resultJSON, _ = json.Marshal(e.Result)
	}

	query := `INSERT INTO experiments (project_id, name, description, status,
		control_version_id, treatment_version_id, traffic_percent, split_ratio,
		target_selector, primary_metric, min_sample_size, confidence_level,
		auto_conclude, max_duration, result, winner_version_id, conclusion_note, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		RETURNING id, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		e.ProjectID, e.Name, e.Description, e.Status,
		e.ControlVersionID, e.TreatmentVersionID, e.TrafficPercent, e.SplitRatio,
		selectorJSON, e.PrimaryMetric, e.MinSampleSize, e.ConfidenceLevel,
		e.AutoConclude, e.MaxDuration, resultJSON, e.WinnerVersionID, e.ConclusionNote, e.CreatedBy,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("experiment.Create: %w: name %q in project", ErrConflict, e.Name)
		}
		return fmt.Errorf("experiment.Create: %w", err)
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Experiment, error) {
	e, err := scanExperiment(r.pool.QueryRow(ctx, `SELECT `+expColumns+` FROM experiments WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("experiment.GetByID: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("experiment.GetByID: %w", err)
	}
	return e, nil
}

func (r *Repository) List(ctx context.Context, projectID *uuid.UUID, limit, offset int) ([]Experiment, int, error) {
	query := `SELECT ` + expColumns + `, COUNT(*) OVER() AS total FROM experiments`
	args := []interface{}{}
	argIdx := 1
	if projectID != nil {
		query += fmt.Sprintf(" WHERE project_id = $%d", argIdx)
		args = append(args, *projectID)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("experiment.List: %w", err)
	}
	defer rows.Close()

	var exps []Experiment
	var total int
	for rows.Next() {
		var e Experiment
		var selectorJSON, resultJSON []byte
		if err := rows.Scan(
			&e.ID, &e.ProjectID, &e.Name, &e.Description, &e.Status,
			&e.ControlVersionID, &e.TreatmentVersionID, &e.TrafficPercent, &e.SplitRatio,
			&selectorJSON, &e.PrimaryMetric, &e.MinSampleSize, &e.ConfidenceLevel,
			&e.AutoConclude, &e.MaxDuration, &resultJSON, &e.WinnerVersionID, &e.ConclusionNote,
			&e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &e.StartedAt, &e.ConcludedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("experiment.List: scanning: %w", err)
		}
		if selectorJSON != nil {
			_ = json.Unmarshal(selectorJSON, &e.TargetSelector)
		}
		if resultJSON != nil {
			e.Result = &ExperimentResult{}
			_ = json.Unmarshal(resultJSON, e.Result)
		}
		exps = append(exps, e)
	}
	return exps, total, rows.Err()
}

func (r *Repository) Update(ctx context.Context, e *Experiment) error {
	selectorJSON, _ := json.Marshal(e.TargetSelector)
	var resultJSON []byte
	if e.Result != nil {
		resultJSON, _ = json.Marshal(e.Result)
	}

	query := `UPDATE experiments SET status=$1, result=$2, winner_version_id=$3,
		conclusion_note=$4, started_at=$5, concluded_at=$6, target_selector=$7
		WHERE id=$8 RETURNING updated_at`

	err := r.pool.QueryRow(ctx, query,
		e.Status, resultJSON, e.WinnerVersionID, e.ConclusionNote,
		e.StartedAt, e.ConcludedAt, selectorJSON, e.ID,
	).Scan(&e.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("experiment.Update: %w", ErrNotFound)
		}
		return fmt.Errorf("experiment.Update: %w", err)
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM experiments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("experiment.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("experiment.Delete: %w", ErrNotFound)
	}
	return nil
}

func (r *Repository) ListByStatus(ctx context.Context, statuses []string) ([]Experiment, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+expColumns+` FROM experiments WHERE status = ANY($1)`, statuses)
	if err != nil {
		return nil, fmt.Errorf("experiment.ListByStatus: %w", err)
	}
	defer rows.Close()

	var exps []Experiment
	for rows.Next() {
		e, err := scanExperiment(rows)
		if err != nil {
			return nil, fmt.Errorf("experiment.ListByStatus: scanning: %w", err)
		}
		exps = append(exps, *e)
	}
	return exps, rows.Err()
}

func (r *Repository) AssignCohort(ctx context.Context, a *CohortAssignment) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO cohort_assignments (device_id, experiment_id, cohort) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
		a.DeviceID, a.ExperimentID, a.Cohort)
	if err != nil {
		return fmt.Errorf("experiment.AssignCohort: %w", err)
	}
	return nil
}

func (r *Repository) GetCohortAssignment(ctx context.Context, deviceID, experimentID uuid.UUID) (*CohortAssignment, error) {
	a := &CohortAssignment{}
	err := r.pool.QueryRow(ctx,
		`SELECT device_id, experiment_id, cohort, assigned_at FROM cohort_assignments WHERE device_id=$1 AND experiment_id=$2`,
		deviceID, experimentID).Scan(&a.DeviceID, &a.ExperimentID, &a.Cohort, &a.AssignedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("experiment.GetCohortAssignment: %w", err)
	}
	return a, nil
}

func (r *Repository) ListCohortAssignments(ctx context.Context, experimentID uuid.UUID) ([]CohortAssignment, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT device_id, experiment_id, cohort, assigned_at FROM cohort_assignments WHERE experiment_id=$1`, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var assignments []CohortAssignment
	for rows.Next() {
		var a CohortAssignment
		if err := rows.Scan(&a.DeviceID, &a.ExperimentID, &a.Cohort, &a.AssignedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}

func (r *Repository) GetCohortStats(ctx context.Context, experimentID, versionID uuid.UUID, since time.Time, cohort string) (*CohortStats, error) {
	query := `SELECT
		COUNT(*) AS sample_size,
		COALESCE(AVG(te.latency_ms), 0),
		COALESCE(STDDEV(te.latency_ms), 0),
		COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY te.latency_ms), 0),
		COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY te.latency_ms), 0),
		COUNT(*) FILTER (WHERE te.error_message IS NOT NULL AND te.error_message != ''),
		COALESCE(AVG(te.confidence_score), 0)
		FROM telemetry_events te
		WHERE te.model_version_id = $2 AND te.recorded_at >= $3
		  AND (te.experiment_arm_id = $5
		       OR (te.experiment_arm_id IS NULL AND EXISTS (
		           SELECT 1 FROM cohort_assignments ca
		           WHERE ca.device_id = te.device_id AND ca.experiment_id = $1 AND ca.cohort = $4)))`

	// Events tagged with this arm's ID count for the arm; untagged events
	// (older SDKs) fall back to the device's cohort assignment.
	cs := &CohortStats{}
	err := r.pool.QueryRow(ctx, query, experimentID, versionID, since, cohort, ArmID(experimentID, cohort)).Scan(
		&cs.SampleSize, &cs.AvgLatency, &cs.StdDevLatency, &cs.P50Latency, &cs.P95Latency,
		&cs.ErrorCount, &cs.AvgConfidence,
	)
	if err != nil {
		return nil, fmt.Errorf("experiment.GetCohortStats: %w", err)
	}
	if cs.SampleSize > 0 {
		cs.ErrorRate = float64(cs.ErrorCount) / float64(cs.SampleSize)
	}
	return cs, nil
}

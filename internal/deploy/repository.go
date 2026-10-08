package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository defines data access for deployments.
type Repository interface {
	Create(ctx context.Context, d *Deployment) error
	GetByID(ctx context.Context, id uuid.UUID) (*Deployment, error)
	List(ctx context.Context, limit, offset int) ([]Deployment, int, error)
	ListByStatus(ctx context.Context, statuses []string) ([]Deployment, error)
	Update(ctx context.Context, d *Deployment) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// Repo implements Repository backed by PostgreSQL.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo creates a new deployment repository.
func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

const deployColumns = `id, project_id, model_version_id, name, target_selector, strategy,
	canary_percentage, canary_stages, current_stage, rolling_config, health_gate,
	previous_version_id, rollback_reason, status, created_by, created_at, updated_at, completed_at,
	stage_started_at, rollout_threshold`

func (r *Repo) Create(ctx context.Context, d *Deployment) error {
	selectorJSON, _ := json.Marshal(d.TargetSelector)
	stagesJSON, _ := json.Marshal(d.CanaryStages)
	var rollingJSON, gateJSON []byte
	if d.RollingConfig != nil {
		rollingJSON, _ = json.Marshal(d.RollingConfig)
	}
	if d.HealthGate != nil {
		gateJSON, _ = json.Marshal(d.HealthGate)
	}

	query := `INSERT INTO deployments (project_id, model_version_id, name, target_selector, strategy,
		canary_percentage, canary_stages, current_stage, rolling_config, health_gate,
		previous_version_id, rollback_reason, status, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query,
		d.ProjectID, d.ModelVersionID, d.Name, selectorJSON, d.Strategy,
		d.CanaryPercentage, stagesJSON, d.CurrentStage, rollingJSON, gateJSON,
		d.PreviousVersionID, d.RollbackReason, d.Status, d.CreatedBy,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("deployment.Create: %w", err)
	}
	return nil
}

func scanDeployment(row pgx.Row) (*Deployment, error) {
	d := &Deployment{}
	var selectorJSON, stagesJSON, rollingJSON, gateJSON []byte
	err := row.Scan(
		&d.ID, &d.ProjectID, &d.ModelVersionID, &d.Name, &selectorJSON, &d.Strategy,
		&d.CanaryPercentage, &stagesJSON, &d.CurrentStage, &rollingJSON, &gateJSON,
		&d.PreviousVersionID, &d.RollbackReason, &d.Status, &d.CreatedBy,
		&d.CreatedAt, &d.UpdatedAt, &d.CompletedAt, &d.StageStartedAt, &d.RolloutThreshold,
	)
	if err != nil {
		return nil, err
	}
	if selectorJSON != nil {
		_ = json.Unmarshal(selectorJSON, &d.TargetSelector)
	}
	if stagesJSON != nil {
		_ = json.Unmarshal(stagesJSON, &d.CanaryStages)
	}
	if rollingJSON != nil {
		d.RollingConfig = &RollingConfig{}
		_ = json.Unmarshal(rollingJSON, d.RollingConfig)
	}
	if gateJSON != nil {
		d.HealthGate = &HealthGate{}
		_ = json.Unmarshal(gateJSON, d.HealthGate)
	}
	return d, nil
}

func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*Deployment, error) {
	query := `SELECT ` + deployColumns + ` FROM deployments WHERE id = $1`
	d, err := scanDeployment(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("deployment.GetByID: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("deployment.GetByID: %w", err)
	}
	return d, nil
}

func (r *Repo) List(ctx context.Context, limit, offset int) ([]Deployment, int, error) {
	query := `SELECT ` + deployColumns + `, COUNT(*) OVER() AS total
		FROM deployments ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("deployment.List: %w", err)
	}
	defer rows.Close()

	var deployments []Deployment
	var total int
	for rows.Next() {
		var d Deployment
		var selectorJSON, stagesJSON, rollingJSON, gateJSON []byte
		if err := rows.Scan(
			&d.ID, &d.ProjectID, &d.ModelVersionID, &d.Name, &selectorJSON, &d.Strategy,
			&d.CanaryPercentage, &stagesJSON, &d.CurrentStage, &rollingJSON, &gateJSON,
			&d.PreviousVersionID, &d.RollbackReason, &d.Status, &d.CreatedBy,
			&d.CreatedAt, &d.UpdatedAt, &d.CompletedAt, &d.StageStartedAt, &d.RolloutThreshold, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("deployment.List: scanning: %w", err)
		}
		if selectorJSON != nil {
			_ = json.Unmarshal(selectorJSON, &d.TargetSelector)
		}
		if stagesJSON != nil {
			_ = json.Unmarshal(stagesJSON, &d.CanaryStages)
		}
		if rollingJSON != nil {
			d.RollingConfig = &RollingConfig{}
			_ = json.Unmarshal(rollingJSON, d.RollingConfig)
		}
		if gateJSON != nil {
			d.HealthGate = &HealthGate{}
			_ = json.Unmarshal(gateJSON, d.HealthGate)
		}
		deployments = append(deployments, d)
	}
	return deployments, total, rows.Err()
}

func (r *Repo) ListByStatus(ctx context.Context, statuses []string) ([]Deployment, error) {
	query := `SELECT ` + deployColumns + ` FROM deployments WHERE status = ANY($1) ORDER BY created_at`
	rows, err := r.pool.Query(ctx, query, statuses)
	if err != nil {
		return nil, fmt.Errorf("deployment.ListByStatus: %w", err)
	}
	defer rows.Close()

	var deployments []Deployment
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, fmt.Errorf("deployment.ListByStatus: scanning: %w", err)
		}
		deployments = append(deployments, *d)
	}
	return deployments, rows.Err()
}

func (r *Repo) Update(ctx context.Context, d *Deployment) error {
	selectorJSON, _ := json.Marshal(d.TargetSelector)
	stagesJSON, _ := json.Marshal(d.CanaryStages)
	var rollingJSON, gateJSON []byte
	if d.RollingConfig != nil {
		rollingJSON, _ = json.Marshal(d.RollingConfig)
	}
	if d.HealthGate != nil {
		gateJSON, _ = json.Marshal(d.HealthGate)
	}

	query := `UPDATE deployments SET
		status = $1, canary_stages = $2, current_stage = $3, rolling_config = $4,
		health_gate = $5, previous_version_id = $6, rollback_reason = $7,
		target_selector = $8, completed_at = $9, stage_started_at = $10, rollout_threshold = $11
		WHERE id = $12 RETURNING updated_at`

	err := r.pool.QueryRow(ctx, query,
		d.Status, stagesJSON, d.CurrentStage, rollingJSON, gateJSON,
		d.PreviousVersionID, d.RollbackReason, selectorJSON, d.CompletedAt,
		d.StageStartedAt, d.RolloutThreshold, d.ID,
	).Scan(&d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("deployment.Update: %w", ErrNotFound)
		}
		return fmt.Errorf("deployment.Update: %w", err)
	}
	return nil
}

func (r *Repo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM deployments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deployment.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deployment.Delete: %w", ErrNotFound)
	}
	return nil
}

package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// TelemetryQuerier provides telemetry data for health gate evaluation.
type TelemetryQuerier interface {
	GetRecentStats(ctx context.Context, modelVersionID uuid.UUID, since time.Time) (*DeploymentStats, error)
}

// DeploymentStats holds telemetry stats for health gate evaluation.
type DeploymentStats struct {
	TotalEvents  int64
	ErrorCount   int64
	ErrorRate    float64
	AvgLatencyMs float64
}

// Valid state transitions. ROLLED_BACK and CANCELLED are terminal.
// Rolling back a COMPLETE deployment reverts devices to the previous version.
var validTransitions = map[string]map[string]bool{
	"PENDING":     {"ROLLING_OUT": true, "CANCELLED": true},
	"ROLLING_OUT": {"PAUSED": true, "COMPLETE": true, "ROLLED_BACK": true, "CANCELLED": true},
	"PAUSED":      {"ROLLING_OUT": true, "COMPLETE": true, "ROLLED_BACK": true, "CANCELLED": true},
	"COMPLETE":    {"ROLLED_BACK": true},
}

const defaultMaxRollingBatches = 10

// Engine manages the lifecycle of deployments.
type Engine struct {
	repo          Repository
	telemetryRepo TelemetryQuerier
	deviceCounter DeviceCounter
	logger        *slog.Logger
}

// NewEngine creates a new deployment engine.
func NewEngine(repo Repository, telemetryRepo TelemetryQuerier, deviceCounter DeviceCounter, logger *slog.Logger) *Engine {
	return &Engine{repo: repo, telemetryRepo: telemetryRepo, deviceCounter: deviceCounter, logger: logger}
}

// StartDeployment transitions PENDING → ROLLING_OUT.
func (e *Engine) StartDeployment(ctx context.Context, deploymentID uuid.UUID) (*Deployment, error) {
	dep, err := e.repo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	if err := e.validateTransition(dep.Status, "ROLLING_OUT"); err != nil {
		return nil, err
	}

	switch dep.Strategy {
	case "ALL_AT_ONCE":
		dep.Status = "COMPLETE"
		now := time.Now()
		dep.CompletedAt = &now
	case "CANARY":
		dep.Status = "ROLLING_OUT"
		if len(dep.CanaryStages) > 0 {
			dep.CurrentStage = 0
			now := time.Now()
			dep.CanaryStages[0].Status = "ACTIVE"
			dep.CanaryStages[0].StartedAt = &now
		}
	case "ROLLING":
		dep.Status = "ROLLING_OUT"
	default:
		dep.Status = "COMPLETE"
		now := time.Now()
		dep.CompletedAt = &now
	}

	if err := e.repo.Update(ctx, dep); err != nil {
		return nil, fmt.Errorf("starting deployment: %w", err)
	}
	if e.logger != nil {
		e.logger.Info("deployment started", "id", dep.ID, "strategy", dep.Strategy, "status", dep.Status)
	}
	return dep, nil
}

// ProgressDeployment evaluates health and advances the rollout.
func (e *Engine) ProgressDeployment(ctx context.Context, deploymentID uuid.UUID) (*Deployment, error) {
	dep, err := e.repo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	if dep.Status != "ROLLING_OUT" {
		return dep, nil // No-op for non-active deployments
	}

	// Evaluate health gate
	healthy, reason := e.EvaluateHealthGate(ctx, dep)
	if !healthy {
		if e.logger != nil {
			e.logger.Warn("health gate failed", "deployment_id", dep.ID, "reason", reason)
		}
		if dep.HealthGate != nil && dep.HealthGate.OnFailure == "PAUSE" {
			return e.doPause(ctx, dep)
		}
		return e.doRollback(ctx, dep, reason)
	}

	switch dep.Strategy {
	case "CANARY":
		return e.progressCanary(ctx, dep)
	case "ROLLING":
		return e.progressRolling(ctx, dep)
	default:
		return dep, nil
	}
}

func (e *Engine) progressCanary(ctx context.Context, dep *Deployment) (*Deployment, error) {
	if dep.CurrentStage >= len(dep.CanaryStages) {
		return dep, nil
	}

	stage := &dep.CanaryStages[dep.CurrentStage]

	// Check min duration
	if stage.StartedAt != nil {
		dur, err := time.ParseDuration(stage.MinDuration)
		if err == nil && dur > 0 {
			if time.Since(*stage.StartedAt) < dur {
				return dep, nil // Not enough time elapsed
			}
		}
	}

	// Mark current stage as passed
	now := time.Now()
	stage.Status = "PASSED"
	stage.CompletedAt = &now

	// Advance to next stage
	dep.CurrentStage++
	if dep.CurrentStage >= len(dep.CanaryStages) {
		// All stages complete
		dep.Status = "COMPLETE"
		dep.CompletedAt = &now
	} else {
		nextStage := &dep.CanaryStages[dep.CurrentStage]
		nextStage.Status = "ACTIVE"
		nextStage.StartedAt = &now
	}

	if err := e.repo.Update(ctx, dep); err != nil {
		return nil, fmt.Errorf("progressing canary: %w", err)
	}
	if e.logger != nil {
		e.logger.Info("canary progressed", "deployment_id", dep.ID, "stage", dep.CurrentStage, "status", dep.Status)
	}
	return dep, nil
}

func (e *Engine) progressRolling(ctx context.Context, dep *Deployment) (*Deployment, error) {
	batchSize := 10
	maxBatches := defaultMaxRollingBatches
	if dep.RollingConfig != nil && dep.RollingConfig.BatchSize > 0 {
		batchSize = dep.RollingConfig.BatchSize
	}

	// Determine total batches needed
	if e.deviceCounter != nil {
		total, err := e.deviceCounter.CountMatchingDevices(ctx, dep.TargetSelector)
		if err == nil && total > 0 {
			maxBatches = (total + batchSize - 1) / batchSize
		}
	}

	// Advance batch
	dep.CurrentStage++
	if dep.CurrentStage >= maxBatches {
		now := time.Now()
		dep.Status = "COMPLETE"
		dep.CompletedAt = &now
	}

	if err := e.repo.Update(ctx, dep); err != nil {
		return nil, fmt.Errorf("progressing rolling: %w", err)
	}
	if e.logger != nil {
		e.logger.Info("rolling progressed", "deployment_id", dep.ID, "batch", dep.CurrentStage, "status", dep.Status)
	}
	return dep, nil
}

// PauseDeployment freezes a rollout.
func (e *Engine) PauseDeployment(ctx context.Context, deploymentID uuid.UUID) (*Deployment, error) {
	dep, err := e.repo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	return e.doPause(ctx, dep)
}

func (e *Engine) doPause(ctx context.Context, dep *Deployment) (*Deployment, error) {
	if err := e.validateTransition(dep.Status, "PAUSED"); err != nil {
		return nil, err
	}
	dep.Status = "PAUSED"
	if err := e.repo.Update(ctx, dep); err != nil {
		return nil, fmt.Errorf("pausing deployment: %w", err)
	}
	if e.logger != nil {
		e.logger.Info("deployment paused", "id", dep.ID)
	}
	return dep, nil
}

// ResumeDeployment continues a paused rollout.
func (e *Engine) ResumeDeployment(ctx context.Context, deploymentID uuid.UUID) (*Deployment, error) {
	dep, err := e.repo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	if err := e.validateTransition(dep.Status, "ROLLING_OUT"); err != nil {
		return nil, err
	}
	dep.Status = "ROLLING_OUT"
	if err := e.repo.Update(ctx, dep); err != nil {
		return nil, fmt.Errorf("resuming deployment: %w", err)
	}
	if e.logger != nil {
		e.logger.Info("deployment resumed", "id", dep.ID)
	}
	return dep, nil
}

// RollbackDeployment reverts to the previous model version.
func (e *Engine) RollbackDeployment(ctx context.Context, deploymentID uuid.UUID, reason string) (*Deployment, error) {
	dep, err := e.repo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	return e.doRollback(ctx, dep, reason)
}

// doRollback marks the deployment ROLLED_BACK. If the previous version is
// known, a COMPLETE revert deployment of it is created so every targeted
// device gets an explicit LOAD/UPDATE back to that version; otherwise devices
// fall back to the next older active deployment of the same model.
func (e *Engine) doRollback(ctx context.Context, dep *Deployment, reason string) (*Deployment, error) {
	if err := e.validateTransition(dep.Status, "ROLLED_BACK"); err != nil {
		return nil, err
	}
	dep.Status = "ROLLED_BACK"
	dep.RollbackReason = reason
	now := time.Now()
	dep.CompletedAt = &now

	// Mark any active canary stage as failed
	for i := range dep.CanaryStages {
		if dep.CanaryStages[i].Status == "ACTIVE" {
			dep.CanaryStages[i].Status = "FAILED"
			dep.CanaryStages[i].CompletedAt = &now
		}
	}

	if err := e.repo.Update(ctx, dep); err != nil {
		return nil, fmt.Errorf("rolling back deployment: %w", err)
	}

	// Create revert deployment if previous version is known
	if dep.PreviousVersionID != nil {
		revert := &Deployment{
			ProjectID:      dep.ProjectID,
			ModelVersionID: *dep.PreviousVersionID,
			Name:           fmt.Sprintf("revert-%s", dep.Name),
			TargetSelector: dep.TargetSelector,
			Strategy:       "ALL_AT_ONCE",
			Status:         "COMPLETE",
			CreatedBy:      "system-rollback",
		}
		revertNow := time.Now()
		revert.CompletedAt = &revertNow
		if err := e.repo.Create(ctx, revert); err != nil && e.logger != nil {
			// The rollback itself stands: devices fall back to the previous active deployment.
			e.logger.Error("creating revert deployment failed", "deployment_id", dep.ID, "error", err)
		}
	}

	if e.logger != nil {
		e.logger.Info("deployment rolled back", "id", dep.ID, "reason", reason)
	}
	return dep, nil
}

// CancelDeployment stops a deployment that has not finished. Its devices fall
// back to the previous active deployment of the same model.
func (e *Engine) CancelDeployment(ctx context.Context, deploymentID uuid.UUID) (*Deployment, error) {
	dep, err := e.repo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	if err := e.validateTransition(dep.Status, "CANCELLED"); err != nil {
		return nil, err
	}
	now := time.Now()
	dep.Status = "CANCELLED"
	dep.CompletedAt = &now
	if err := e.repo.Update(ctx, dep); err != nil {
		return nil, fmt.Errorf("cancelling deployment: %w", err)
	}
	if e.logger != nil {
		e.logger.Info("deployment cancelled", "id", dep.ID)
	}
	return dep, nil
}

// CompleteDeployment promotes a rolling-out or paused deployment to every
// targeted device immediately.
func (e *Engine) CompleteDeployment(ctx context.Context, deploymentID uuid.UUID) (*Deployment, error) {
	dep, err := e.repo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	if err := e.validateTransition(dep.Status, "COMPLETE"); err != nil {
		return nil, err
	}
	now := time.Now()
	dep.Status = "COMPLETE"
	dep.CompletedAt = &now
	if err := e.repo.Update(ctx, dep); err != nil {
		return nil, fmt.Errorf("completing deployment: %w", err)
	}
	return dep, nil
}

// EvaluateHealthGate checks telemetry against the deployment's health gate.
func (e *Engine) EvaluateHealthGate(ctx context.Context, dep *Deployment) (healthy bool, reason string) {
	if dep.HealthGate == nil {
		return true, "no health gate configured"
	}

	gate := dep.HealthGate
	stats, err := e.telemetryRepo.GetRecentStats(ctx, dep.ModelVersionID, time.Now().Add(-10*time.Minute))
	if err != nil {
		return true, fmt.Sprintf("telemetry query failed: %v (passing by default)", err)
	}

	if stats.TotalEvents < int64(gate.MinEvents) {
		return true, fmt.Sprintf("insufficient data: %d/%d events", stats.TotalEvents, gate.MinEvents)
	}

	if stats.ErrorRate > gate.MaxErrorRate {
		return false, fmt.Sprintf("error rate %.2f%% exceeds threshold %.2f%%",
			stats.ErrorRate*100, gate.MaxErrorRate*100)
	}

	return true, "all checks passed"
}

// GetHealthStatus returns the health gate status for a deployment.
func (e *Engine) GetHealthStatus(ctx context.Context, deploymentID uuid.UUID) (map[string]interface{}, error) {
	dep, err := e.repo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	healthy, reason := e.EvaluateHealthGate(ctx, dep)

	stagePct := 100
	if dep.Strategy == "CANARY" && dep.CurrentStage < len(dep.CanaryStages) {
		stagePct = dep.CanaryStages[dep.CurrentStage].Percentage
	}

	canProgress := dep.Status == "ROLLING_OUT" && healthy
	result := map[string]interface{}{
		"deployment_id":    dep.ID,
		"status":           dep.Status,
		"current_stage":    dep.CurrentStage,
		"stage_percentage": stagePct,
		"healthy":          healthy,
		"reason":           reason,
		"can_progress":     canProgress,
	}
	return result, nil
}

func (e *Engine) validateTransition(from, to string) error {
	allowed, ok := validTransitions[from]
	if !ok || !allowed[to] {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, from, to)
	}
	return nil
}

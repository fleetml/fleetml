package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/alert"
	"github.com/fleetml/fleetml/internal/rollout"
)

// TelemetryQuerier provides telemetry for health gate evaluation.
type TelemetryQuerier interface {
	// CohortStats aggregates a version's events from the given devices since a time.
	CohortStats(ctx context.Context, modelVersionID uuid.UUID, deviceIDs []uuid.UUID, since time.Time) (*DeploymentStats, error)
	// VersionP95 is a version's p95 inference latency over all devices since a time.
	VersionP95(ctx context.Context, modelVersionID uuid.UUID, since time.Time) (p95 float64, events int64, err error)
}

// DeploymentStats holds telemetry stats for health gate evaluation.
type DeploymentStats struct {
	TotalEvents  int64   `json:"total_events"`
	ErrorCount   int64   `json:"error_count"`
	ErrorRate    float64 `json:"error_rate"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P95LatencyMs float64 `json:"p95_latency_ms"`
}

// GateResult is the outcome of a health gate evaluation.
type GateResult struct {
	Healthy bool
	// Hold means the gate could not be evaluated (e.g. a query failed): the
	// rollout neither advances nor rolls back until it can be.
	Hold   bool
	Reason string
	Stats  *DeploymentStats
	Cohort int
}

// latencyBaselineWindow is how far back the previous version's p95 is measured.
const latencyBaselineWindow = 24 * time.Hour

// Valid state transitions. ROLLED_BACK and CANCELLED are terminal.
// Rolling back a COMPLETE deployment reverts devices to the previous version.
var validTransitions = map[string]map[string]bool{
	"PENDING":     {"ROLLING_OUT": true, "CANCELLED": true},
	"ROLLING_OUT": {"PAUSED": true, "COMPLETE": true, "ROLLED_BACK": true, "CANCELLED": true},
	"PAUSED":      {"ROLLING_OUT": true, "COMPLETE": true, "ROLLED_BACK": true, "CANCELLED": true},
	"COMPLETE":    {"ROLLED_BACK": true},
}

// unavailableSeenWithin limits max_unavailable to devices that are online:
// an offline device must not hold a rollout forever.
const unavailableSeenWithin = 5 * time.Minute

// AlertSender delivers alerts (implemented by *alert.Notifier).
type AlertSender interface {
	Send(a alert.Alert)
}

// Engine manages the lifecycle of deployments.
type Engine struct {
	repo          Repository
	telemetryRepo TelemetryQuerier
	devices       DeviceDirectory
	alerts        AlertSender
	logger        *slog.Logger
}

// SetAlertSender makes every rollback send a rollback alert.
func (e *Engine) SetAlertSender(a AlertSender) {
	e.alerts = a
}

// NewEngine creates a new deployment engine.
func NewEngine(repo Repository, telemetryRepo TelemetryQuerier, devices DeviceDirectory, logger *slog.Logger) *Engine {
	return &Engine{repo: repo, telemetryRepo: telemetryRepo, devices: devices, logger: logger}
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
			dep.StageStartedAt = &now
		}
	case "ROLLING":
		dep.Status = "ROLLING_OUT"
		dep.CurrentStage = 1
		now := time.Now()
		dep.StageStartedAt = &now
		threshold, err := e.rollingThreshold(ctx, dep, 1)
		if err != nil {
			return nil, fmt.Errorf("computing first batch: %w", err)
		}
		dep.RolloutThreshold = &threshold
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

	gate := e.EvaluateHealthGate(ctx, dep)
	if gate.Hold {
		if e.logger != nil {
			e.logger.Warn("health gate could not be evaluated; holding rollout", "deployment_id", dep.ID, "reason", gate.Reason)
		}
		return dep, nil
	}
	if !gate.Healthy {
		if e.logger != nil {
			e.logger.Warn("health gate failed", "deployment_id", dep.ID, "reason", gate.Reason)
		}
		if effectiveGate(dep).OnFailure == "PAUSE" {
			return e.doPause(ctx, dep)
		}
		return e.doRollback(ctx, dep, gate.Reason, "health_gate")
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
		dep.StageStartedAt = &now
	}

	if err := e.repo.Update(ctx, dep); err != nil {
		return nil, fmt.Errorf("progressing canary: %w", err)
	}
	if e.logger != nil {
		e.logger.Info("canary progressed", "deployment_id", dep.ID, "stage", dep.CurrentStage, "status", dep.Status)
	}
	return dep, nil
}

// progressRolling advances a rolling deployment by one batch once the batch
// interval has elapsed since the stage started (the health gate has already
// passed), unless more than max_unavailable exposed devices have not yet
// reported the new version loaded. When every targeted device is exposed
// and the interval has elapsed again, the deployment completes.
func (e *Engine) progressRolling(ctx context.Context, dep *Deployment) (*Deployment, error) {
	rc := dep.RollingConfig
	if rc == nil {
		rc = DefaultRollingConfig()
	}
	interval, err := time.ParseDuration(rc.BatchInterval)
	if err != nil {
		interval = 0
	}
	if dep.StageStartedAt != nil && time.Since(*dep.StageStartedAt) < interval {
		return dep, nil
	}

	if rc.MaxUnavailable > 0 {
		cohort, err := e.cohort(ctx, dep)
		if err != nil {
			return dep, nil // hold: cannot tell which devices are exposed
		}
		unavailable, err := e.devices.CountUnavailable(ctx, cohort, dep.ModelVersionID, time.Now().Add(-unavailableSeenWithin))
		if err != nil {
			return dep, nil // hold
		}
		if unavailable > rc.MaxUnavailable {
			if e.logger != nil {
				e.logger.Info("rolling deployment held: too many unavailable devices",
					"deployment_id", dep.ID, "unavailable", unavailable, "max_unavailable", rc.MaxUnavailable)
			}
			return dep, nil
		}
	}

	now := time.Now()
	if dep.RolloutThreshold != nil && *dep.RolloutThreshold >= rollout.RingSize {
		dep.Status = "COMPLETE"
		dep.CompletedAt = &now
	} else {
		next := dep.CurrentStage + 1
		threshold, err := e.rollingThreshold(ctx, dep, next)
		if err != nil {
			return dep, nil // hold until the device list can be read
		}
		dep.CurrentStage = next
		dep.RolloutThreshold = &threshold
		dep.StageStartedAt = &now
	}

	if err := e.repo.Update(ctx, dep); err != nil {
		return nil, fmt.Errorf("progressing rolling: %w", err)
	}
	if e.logger != nil {
		e.logger.Info("rolling progressed", "deployment_id", dep.ID, "stage", dep.CurrentStage,
			"exposed_pct", dep.EffectiveCanaryPercent(), "status", dep.Status)
	}
	return dep, nil
}

// rollingThreshold is the ring threshold that exposes stage batches.
// Percentage batches map directly onto the ring. Device-count batches sort
// the targeted devices by ring position and cut after stage x batch_size
// devices, so exactly that many current devices are exposed.
func (e *Engine) rollingThreshold(ctx context.Context, dep *Deployment, stage int) (int64, error) {
	rc := dep.RollingConfig
	if rc == nil {
		rc = DefaultRollingConfig()
	}
	if rc.BatchSizePercent > 0 {
		return rollout.PercentThreshold(stage * rc.BatchSizePercent), nil
	}
	if e.devices == nil {
		return 0, fmt.Errorf("no device directory configured")
	}
	targeted, err := e.devices.ListMatchingDevices(ctx, dep.ProjectID, dep.TargetSelector)
	if err != nil {
		return 0, err
	}
	want := stage * rc.BatchSize
	if want >= len(targeted) {
		return rollout.RingSize, nil
	}
	positions := make([]int64, len(targeted))
	for i, id := range targeted {
		positions[i] = int64(rollout.Position(id, dep.ID))
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i] < positions[j] })
	return positions[want-1] + 1, nil
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
	return e.doRollback(ctx, dep, reason, "manual")
}

// doRollback marks the deployment ROLLED_BACK. If the previous version is
// known, a COMPLETE revert deployment of it is created so every targeted
// device gets an explicit LOAD/UPDATE back to that version; otherwise devices
// fall back to the next older active deployment of the same model.
func (e *Engine) doRollback(ctx context.Context, dep *Deployment, reason, trigger string) (*Deployment, error) {
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
			Name:           revertName(dep),
			TargetSelector: dep.TargetSelector,
			Strategy:       "ALL_AT_ONCE",
			Status:         "COMPLETE",
			CreatedBy:      "system-rollback",
			// Getting devices off a failing version must not wait for Wi-Fi.
			Urgent: true,
		}
		revertNow := time.Now()
		revert.CompletedAt = &revertNow
		if err := e.repo.Create(ctx, revert); err != nil && e.logger != nil {
			// The rollback itself stands: devices fall back to the previous active deployment.
			e.logger.Error("creating revert deployment failed", "deployment_id", dep.ID, "error", err)
		}
	}

	if e.logger != nil {
		e.logger.Info("deployment rolled back", "id", dep.ID, "reason", reason, "trigger", trigger)
	}
	if e.alerts != nil {
		rb := &alert.Rollback{
			DeploymentID: dep.ID.String(), ProjectID: dep.ProjectID.String(), ModelVersionID: dep.ModelVersionID.String(),
			Strategy: dep.Strategy, Stage: dep.CurrentStage, Trigger: trigger, Reason: reason,
		}
		if dep.PreviousVersionID != nil {
			rb.PreviousVersionID = dep.PreviousVersionID.String()
		}
		e.alerts.Send(alert.Alert{Type: alert.TypeRollback, Rollback: rb})
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

func effectiveGate(dep *Deployment) *HealthGate {
	if dep.HealthGate != nil {
		return dep.HealthGate
	}
	return DefaultHealthGate()
}

// cohort returns the targeted devices the deployment currently exposes.
func (e *Engine) cohort(ctx context.Context, dep *Deployment) ([]uuid.UUID, error) {
	if e.devices == nil {
		return nil, fmt.Errorf("no device directory configured")
	}
	targeted, err := e.devices.ListMatchingDevices(ctx, dep.ProjectID, dep.TargetSelector)
	if err != nil {
		return nil, err
	}
	exposed := make([]uuid.UUID, 0, len(targeted))
	for _, id := range targeted {
		if dep.Exposes(id) {
			exposed = append(exposed, id)
		}
	}
	return exposed, nil
}

// EvaluateHealthGate judges the current stage on telemetry from the devices
// in its cohort only, since the stage started. Deployments without a gate
// use DefaultHealthGate. Any query error holds the rollout (fails closed).
func (e *Engine) EvaluateHealthGate(ctx context.Context, dep *Deployment) GateResult {
	gate := effectiveGate(dep)
	if e.telemetryRepo == nil {
		return GateResult{Hold: true, Reason: "no telemetry source configured"}
	}
	cohort, err := e.cohort(ctx, dep)
	if err != nil {
		return GateResult{Hold: true, Reason: fmt.Sprintf("listing cohort failed: %v", err)}
	}
	since := dep.stageStartedAt()
	stats, err := e.telemetryRepo.CohortStats(ctx, dep.ModelVersionID, cohort, since)
	if err != nil {
		return GateResult{Hold: true, Reason: fmt.Sprintf("telemetry query failed: %v", err), Cohort: len(cohort)}
	}
	res := GateResult{Stats: stats, Cohort: len(cohort)}

	if stats.TotalEvents < int64(gate.MinEvents) {
		res.Healthy = true
		res.Reason = fmt.Sprintf("insufficient data: %d/%d events from %s", stats.TotalEvents, gate.MinEvents, cohortDevices(len(cohort)))
		return res
	}
	if stats.ErrorRate > gate.MaxErrorRate {
		res.Reason = fmt.Sprintf("error rate %.2f%% exceeds threshold %.2f%% (%d/%d events from %s)",
			stats.ErrorRate*100, gate.MaxErrorRate*100, stats.ErrorCount, stats.TotalEvents, cohortDevices(len(cohort)))
		return res
	}
	if gate.MaxLatencyRegression > 0 && dep.PreviousVersionID != nil && stats.P95LatencyMs > 0 {
		prevP95, prevEvents, err := e.telemetryRepo.VersionP95(ctx, *dep.PreviousVersionID, time.Now().Add(-latencyBaselineWindow))
		if err != nil {
			return GateResult{Hold: true, Reason: fmt.Sprintf("baseline latency query failed: %v", err), Stats: stats, Cohort: len(cohort)}
		}
		if prevEvents >= int64(gate.MinEvents) && prevP95 > 0 {
			limit := prevP95 * (1 + gate.MaxLatencyRegression)
			if stats.P95LatencyMs > limit {
				res.Reason = fmt.Sprintf("p95 latency %.0fms is %.0f%% above the previous version's %.0fms (limit +%.0f%%)",
					stats.P95LatencyMs, (stats.P95LatencyMs/prevP95-1)*100, prevP95, gate.MaxLatencyRegression*100)
				return res
			}
		}
	}
	res.Healthy = true
	res.Reason = "all checks passed"
	return res
}

// GetHealthStatus returns the health gate status for a deployment.
func (e *Engine) GetHealthStatus(ctx context.Context, deploymentID uuid.UUID) (map[string]interface{}, error) {
	dep, err := e.repo.GetByID(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	gate := e.EvaluateHealthGate(ctx, dep)

	stagePct := dep.EffectiveCanaryPercent()

	canProgress := dep.Status == "ROLLING_OUT" && gate.Healthy && !gate.Hold
	result := map[string]interface{}{
		"deployment_id":    dep.ID,
		"status":           dep.Status,
		"current_stage":    dep.CurrentStage,
		"stage_percentage": stagePct,
		"healthy":          gate.Healthy,
		"hold":             gate.Hold,
		"reason":           gate.Reason,
		"can_progress":     canProgress,
		"cohort_devices":   gate.Cohort,
		"health_gate":      effectiveGate(dep),
		"stats":            gate.Stats,
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

func cohortDevices(n int) string {
	if n == 1 {
		return "1 cohort device"
	}
	return fmt.Sprintf("%d cohort devices", n)
}

// revertName names the deployment that undoes dep: "revert-<name>", or
// "revert-<id prefix>" for an unnamed deployment.
func revertName(dep *Deployment) string {
	if dep.Name != "" {
		return "revert-" + dep.Name
	}
	return "revert-" + dep.ID.String()[:8]
}

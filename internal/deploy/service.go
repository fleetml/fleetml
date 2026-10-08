package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	validStrategies = map[string]bool{"ALL_AT_ONCE": true, "CANARY": true, "ROLLING": true}
	validStatuses   = map[string]bool{"PENDING": true, "ROLLING_OUT": true, "PAUSED": true, "COMPLETE": true, "ROLLED_BACK": true, "FAILED": true, "CANCELLED": true}
)

// VersionChecker answers questions about model versions for new deployments.
type VersionChecker interface {
	// ReadyVariantCount is the number of READY (sha256-verified) variants;
	// a version without one cannot be rolled out.
	ReadyVariantCount(ctx context.Context, versionID uuid.UUID) (int, error)
	// CurrentVersion is the version of the newest COMPLETE deployment of the
	// same model other than versionID, or nil if there is none.
	CurrentVersion(ctx context.Context, projectID, versionID uuid.UUID) (*uuid.UUID, error)
}

// Service orchestrates deployment business logic.
type Service struct {
	repo     Repository
	engine   *Engine
	versions VersionChecker
	logger   *slog.Logger
}

// SetVersionChecker makes Create reject versions without a READY variant.
func (s *Service) SetVersionChecker(v VersionChecker) {
	s.versions = v
}

// requireDeployable returns ErrNotDeployable if the version has no READY variant.
func requireDeployable(ctx context.Context, v VersionChecker, versionID uuid.UUID) error {
	if v == nil {
		return nil
	}
	n, err := v.ReadyVariantCount(ctx, versionID)
	if err != nil {
		return fmt.Errorf("checking variants: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: model version %s has no READY variant (upload a blob and finalize it first)", ErrNotDeployable, versionID)
	}
	return nil
}

// NewService creates a new deployment service.
func NewService(repo Repository, logger *slog.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

// SetEngine sets the deployment engine.
func (s *Service) SetEngine(engine *Engine) {
	s.engine = engine
}

// CreateInput holds parameters for creating a deployment.
type CreateInput struct {
	ProjectID      uuid.UUID
	ModelVersionID uuid.UUID
	Name           string
	TargetSelector map[string]string
	Strategy       string
	CanaryStages   []CanaryStageInput
	RollingConfig  *RollingConfig
	HealthGate     *HealthGate
	CreatedBy      string
}

// CanaryStageInput is input for canary stage creation.
type CanaryStageInput struct {
	Percentage  int    `json:"percentage"`
	MinDuration string `json:"min_duration"`
}

// Create validates and creates a new deployment.
func (s *Service) Create(ctx context.Context, input CreateInput) (*Deployment, error) {
	strategy := strings.ToUpper(strings.TrimSpace(input.Strategy))
	if strategy == "" {
		strategy = "ALL_AT_ONCE"
	}
	if !validStrategies[strategy] {
		return nil, fmt.Errorf("%w: strategy must be one of ALL_AT_ONCE, CANARY, ROLLING", ErrInvalidInput)
	}

	if err := requireDeployable(ctx, s.versions, input.ModelVersionID); err != nil {
		return nil, err
	}
	var previous *uuid.UUID
	if s.versions != nil {
		p, err := s.versions.CurrentVersion(ctx, input.ProjectID, input.ModelVersionID)
		if err != nil {
			return nil, fmt.Errorf("finding current version: %w", err)
		}
		previous = p
	}

	if input.TargetSelector == nil {
		input.TargetSelector = map[string]string{}
	}
	if input.CreatedBy == "" {
		input.CreatedBy = "system"
	}

	d := &Deployment{
		ProjectID:         input.ProjectID,
		ModelVersionID:    input.ModelVersionID,
		Name:              input.Name,
		TargetSelector:    input.TargetSelector,
		Strategy:          strategy,
		Status:            "PENDING",
		CreatedBy:         input.CreatedBy,
		PreviousVersionID: previous,
	}

	// Build canary stages
	if strategy == "CANARY" {
		if len(input.CanaryStages) == 0 {
			d.CanaryStages = DefaultCanaryStages()
		} else {
			stages := make([]CanaryStage, len(input.CanaryStages))
			for i, s := range input.CanaryStages {
				if s.Percentage <= 0 || s.Percentage > 100 {
					return nil, fmt.Errorf("%w: canary stage percentage must be 1-100", ErrInvalidInput)
				}
				if i > 0 && s.Percentage <= input.CanaryStages[i-1].Percentage {
					return nil, fmt.Errorf("%w: canary stages must have increasing percentages", ErrInvalidInput)
				}
				dur := s.MinDuration
				if dur == "" {
					dur = "0s"
				}
				if _, err := time.ParseDuration(dur); err != nil {
					return nil, fmt.Errorf("%w: invalid min_duration %q: %v", ErrInvalidInput, dur, err)
				}
				stages[i] = CanaryStage{Percentage: s.Percentage, MinDuration: dur, Status: "PENDING"}
			}
			if stages[len(stages)-1].Percentage != 100 {
				return nil, fmt.Errorf("%w: last canary stage must be 100%%", ErrInvalidInput)
			}
			d.CanaryStages = stages
		}
	}

	// Rolling config
	if strategy == "ROLLING" {
		rc := input.RollingConfig
		if rc == nil {
			rc = DefaultRollingConfig()
		}
		if (rc.BatchSize > 0) == (rc.BatchSizePercent > 0) {
			return nil, fmt.Errorf("%w: set exactly one of batch_size (devices) or batch_size_percent", ErrInvalidInput)
		}
		if rc.BatchSize < 0 || rc.BatchSizePercent < 0 || rc.BatchSizePercent > 100 || rc.MaxUnavailable < 0 {
			return nil, fmt.Errorf("%w: batch_size, batch_size_percent (1-100) and max_unavailable must not be negative", ErrInvalidInput)
		}
		if rc.BatchInterval == "" {
			rc.BatchInterval = DefaultRollingConfig().BatchInterval
		}
		if _, err := time.ParseDuration(rc.BatchInterval); err != nil {
			return nil, fmt.Errorf("%w: invalid batch_interval %q: %v", ErrInvalidInput, rc.BatchInterval, err)
		}
		d.RollingConfig = rc
	}

	// Health gate: canary and rolling deployments always get one.
	if input.HealthGate == nil && (strategy == "CANARY" || strategy == "ROLLING") {
		input.HealthGate = DefaultHealthGate()
	}
	if input.HealthGate != nil {
		if input.HealthGate.MaxErrorRate < 0 || input.HealthGate.MaxErrorRate > 1 {
			return nil, fmt.Errorf("%w: max_error_rate must be 0-1", ErrInvalidInput)
		}
		if input.HealthGate.MaxLatencyRegression < 0 {
			return nil, fmt.Errorf("%w: max_latency_regression must be >= 0", ErrInvalidInput)
		}
		if input.HealthGate.MinEvents <= 0 {
			input.HealthGate.MinEvents = DefaultHealthGate().MinEvents
		}
		if input.HealthGate.OnFailure == "" {
			input.HealthGate.OnFailure = "ROLLBACK"
		}
		if input.HealthGate.OnFailure != "PAUSE" && input.HealthGate.OnFailure != "ROLLBACK" {
			return nil, fmt.Errorf("%w: on_failure must be PAUSE or ROLLBACK", ErrInvalidInput)
		}
		d.HealthGate = input.HealthGate
	}

	if err := s.repo.Create(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// Get returns a deployment by ID.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Deployment, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns paginated deployments.
func (s *Service) List(ctx context.Context, limit, offset int) ([]Deployment, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.List(ctx, limit, offset)
}

// UpdateStatus moves a deployment to a new status through the state machine,
// with the same side effects as the dedicated endpoints.
func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, status, reason string) (*Deployment, error) {
	if s.engine == nil {
		return nil, fmt.Errorf("deployment engine not configured")
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if !validStatuses[status] {
		return nil, fmt.Errorf("%w: invalid status %q", ErrInvalidInput, status)
	}
	switch status {
	case "ROLLING_OUT":
		dep, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if dep.Status == "PENDING" {
			return s.engine.StartDeployment(ctx, id)
		}
		return s.engine.ResumeDeployment(ctx, id)
	case "PAUSED":
		return s.engine.PauseDeployment(ctx, id)
	case "COMPLETE":
		return s.engine.CompleteDeployment(ctx, id)
	case "ROLLED_BACK":
		if reason == "" {
			reason = "manual rollback"
		}
		return s.engine.RollbackDeployment(ctx, id, reason)
	case "CANCELLED":
		return s.engine.CancelDeployment(ctx, id)
	default:
		return nil, fmt.Errorf("%w: cannot set status %s directly", ErrInvalidTransition, status)
	}
}

// CancelDeployment delegates to the engine.
func (s *Service) CancelDeployment(ctx context.Context, id uuid.UUID) (*Deployment, error) {
	if s.engine == nil {
		return nil, fmt.Errorf("deployment engine not configured")
	}
	return s.engine.CancelDeployment(ctx, id)
}

// Delete removes a deployment.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// StartDeployment delegates to the engine.
func (s *Service) StartDeployment(ctx context.Context, id uuid.UUID) (*Deployment, error) {
	if s.engine == nil {
		return nil, fmt.Errorf("deployment engine not configured")
	}
	return s.engine.StartDeployment(ctx, id)
}

// ProgressDeployment delegates to the engine.
func (s *Service) ProgressDeployment(ctx context.Context, id uuid.UUID) (*Deployment, error) {
	if s.engine == nil {
		return nil, fmt.Errorf("deployment engine not configured")
	}
	return s.engine.ProgressDeployment(ctx, id)
}

// PauseDeployment delegates to the engine.
func (s *Service) PauseDeployment(ctx context.Context, id uuid.UUID) (*Deployment, error) {
	if s.engine == nil {
		return nil, fmt.Errorf("deployment engine not configured")
	}
	return s.engine.PauseDeployment(ctx, id)
}

// ResumeDeployment delegates to the engine.
func (s *Service) ResumeDeployment(ctx context.Context, id uuid.UUID) (*Deployment, error) {
	if s.engine == nil {
		return nil, fmt.Errorf("deployment engine not configured")
	}
	return s.engine.ResumeDeployment(ctx, id)
}

// RollbackDeployment delegates to the engine.
func (s *Service) RollbackDeployment(ctx context.Context, id uuid.UUID, reason string) (*Deployment, error) {
	if s.engine == nil {
		return nil, fmt.Errorf("deployment engine not configured")
	}
	return s.engine.RollbackDeployment(ctx, id, reason)
}

// GetHealthStatus delegates to the engine.
func (s *Service) GetHealthStatus(ctx context.Context, id uuid.UUID) (map[string]interface{}, error) {
	if s.engine == nil {
		return nil, fmt.Errorf("deployment engine not configured")
	}
	return s.engine.GetHealthStatus(ctx, id)
}

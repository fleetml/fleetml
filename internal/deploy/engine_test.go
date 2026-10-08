package deploy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockRepo struct {
	deployments map[uuid.UUID]*Deployment
}

func newMockRepo() *mockRepo {
	return &mockRepo{deployments: make(map[uuid.UUID]*Deployment)}
}

func (m *mockRepo) Create(_ context.Context, d *Deployment) error {
	d.ID = uuid.New()
	d.CreatedAt = time.Now()
	d.UpdatedAt = time.Now()
	m.deployments[d.ID] = d
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id uuid.UUID) (*Deployment, error) {
	d, ok := m.deployments[id]
	if !ok {
		return nil, fmt.Errorf("deployment.GetByID: %w", ErrNotFound)
	}
	return d, nil
}

func (m *mockRepo) List(_ context.Context, limit, offset int) ([]Deployment, int, error) {
	var all []Deployment
	for _, d := range m.deployments {
		all = append(all, *d)
	}
	total := len(all)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (m *mockRepo) ListByStatus(_ context.Context, statuses []string) ([]Deployment, error) {
	statusSet := make(map[string]bool)
	for _, s := range statuses {
		statusSet[s] = true
	}
	var result []Deployment
	for _, d := range m.deployments {
		if statusSet[d.Status] {
			result = append(result, *d)
		}
	}
	return result, nil
}

func (m *mockRepo) Update(_ context.Context, d *Deployment) error {
	if _, ok := m.deployments[d.ID]; !ok {
		return fmt.Errorf("deployment.Update: %w", ErrNotFound)
	}
	d.UpdatedAt = time.Now()
	m.deployments[d.ID] = d
	return nil
}

func (m *mockRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.deployments[id]; !ok {
		return fmt.Errorf("deployment.Delete: %w", ErrNotFound)
	}
	delete(m.deployments, id)
	return nil
}

type mockTelemetry struct {
	stats *DeploymentStats
	err   error
}

func (m *mockTelemetry) GetRecentStats(_ context.Context, _ uuid.UUID, _ time.Time) (*DeploymentStats, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.stats, nil
}

// --- Helpers ---

func createPendingDep(repo *mockRepo, strategy string, stages []CanaryStage, gate *HealthGate) *Deployment {
	d := &Deployment{
		ProjectID:      uuid.New(),
		ModelVersionID: uuid.New(),
		Strategy:       strategy,
		Status:         "PENDING",
		CanaryStages:   stages,
		HealthGate:     gate,
		TargetSelector: map[string]string{},
		CreatedBy:      "test",
	}
	_ = repo.Create(context.Background(), d)
	return d
}

func quickStages() []CanaryStage {
	return []CanaryStage{
		{Percentage: 1, MinDuration: "0s", Status: "PENDING"},
		{Percentage: 10, MinDuration: "0s", Status: "PENDING"},
		{Percentage: 50, MinDuration: "0s", Status: "PENDING"},
		{Percentage: 100, MinDuration: "0s", Status: "PENDING"},
	}
}

// --- Tests ---

func TestEngine_AllAtOnce_Start(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "ALL_AT_ONCE", nil, nil)

	result, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "COMPLETE", result.Status)
	assert.NotNil(t, result.CompletedAt)
}

func TestEngine_Canary_FullProgression(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)

	// Start
	result, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "ROLLING_OUT", result.Status)
	assert.Equal(t, 0, result.CurrentStage)
	assert.Equal(t, "ACTIVE", result.CanaryStages[0].Status)

	// Progress through all stages
	for i := 0; i < 4; i++ {
		result, err = engine.ProgressDeployment(context.Background(), dep.ID)
		require.NoError(t, err)
	}
	assert.Equal(t, "COMPLETE", result.Status)
}

func TestEngine_Canary_HealthGateRollback(t *testing.T) {
	repo := newMockRepo()
	tel := &mockTelemetry{stats: &DeploymentStats{TotalEvents: 200, ErrorCount: 20, ErrorRate: 0.10}}
	engine := NewEngine(repo, tel, nil, nil)

	gate := &HealthGate{MaxErrorRate: 0.05, MinEvents: 100, OnFailure: "ROLLBACK"}
	dep := createPendingDep(repo, "CANARY", quickStages(), gate)

	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	result, err := engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "ROLLED_BACK", result.Status)
	assert.Contains(t, result.RollbackReason, "error rate")
}

func TestEngine_Canary_HealthGatePause(t *testing.T) {
	repo := newMockRepo()
	tel := &mockTelemetry{stats: &DeploymentStats{TotalEvents: 200, ErrorCount: 20, ErrorRate: 0.10}}
	engine := NewEngine(repo, tel, nil, nil)

	gate := &HealthGate{MaxErrorRate: 0.05, MinEvents: 100, OnFailure: "PAUSE"}
	dep := createPendingDep(repo, "CANARY", quickStages(), gate)

	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	result, err := engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "PAUSED", result.Status)
}

func TestEngine_Canary_InsufficientData(t *testing.T) {
	repo := newMockRepo()
	tel := &mockTelemetry{stats: &DeploymentStats{TotalEvents: 5, ErrorCount: 3, ErrorRate: 0.60}}
	engine := NewEngine(repo, tel, nil, nil)

	gate := &HealthGate{MaxErrorRate: 0.05, MinEvents: 100, OnFailure: "ROLLBACK"}
	dep := createPendingDep(repo, "CANARY", quickStages(), gate)

	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	// Should NOT rollback because insufficient data
	result, err := engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.NotEqual(t, "ROLLED_BACK", result.Status)
}

func TestEngine_Canary_MinDurationNotMet(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)

	stages := []CanaryStage{
		{Percentage: 50, MinDuration: "24h", Status: "PENDING"},
		{Percentage: 100, MinDuration: "0s", Status: "PENDING"},
	}
	dep := createPendingDep(repo, "CANARY", stages, nil)

	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	// Progress should be no-op because min_duration not met
	result, err := engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "ROLLING_OUT", result.Status)
	assert.Equal(t, 0, result.CurrentStage)
}

func TestEngine_PauseAndResume(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)

	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	result, err := engine.PauseDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "PAUSED", result.Status)

	result, err = engine.ResumeDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "ROLLING_OUT", result.Status)
}

func TestEngine_ManualRollback(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)

	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	result, err := engine.RollbackDeployment(context.Background(), dep.ID, "bad performance")
	require.NoError(t, err)
	assert.Equal(t, "ROLLED_BACK", result.Status)
	assert.Equal(t, "bad performance", result.RollbackReason)
}

func TestEngine_RollbackFromPaused(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)

	_, _ = engine.StartDeployment(context.Background(), dep.ID)
	_, _ = engine.PauseDeployment(context.Background(), dep.ID)

	result, err := engine.RollbackDeployment(context.Background(), dep.ID, "decided to rollback")
	require.NoError(t, err)
	assert.Equal(t, "ROLLED_BACK", result.Status)
}

func TestEngine_InvalidTransition_StartComplete(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "ALL_AT_ONCE", nil, nil)

	_, _ = engine.StartDeployment(context.Background(), dep.ID) // COMPLETE

	_, err := engine.StartDeployment(context.Background(), dep.ID)
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestEngine_InvalidTransition_PausePending(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)

	_, err := engine.PauseDeployment(context.Background(), dep.ID) // Can't pause PENDING
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestEngine_InvalidTransition_RollbackPending(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)

	_, err := engine.RollbackDeployment(context.Background(), dep.ID, "test")
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestEngine_ProgressComplete_NoOp(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "ALL_AT_ONCE", nil, nil)

	_, _ = engine.StartDeployment(context.Background(), dep.ID)

	result, err := engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "COMPLETE", result.Status) // No-op
}

func TestEngine_Rolling_Start(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)

	dep := &Deployment{
		ProjectID:      uuid.New(),
		ModelVersionID: uuid.New(),
		Strategy:       "ROLLING",
		Status:         "PENDING",
		RollingConfig:  &RollingConfig{BatchSize: 10, BatchInterval: "1m", MaxUnavailable: 10},
		TargetSelector: map[string]string{},
		CreatedBy:      "test",
	}
	_ = repo.Create(context.Background(), dep)

	result, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "ROLLING_OUT", result.Status)
}

func TestEngine_HealthGate_NoGate(t *testing.T) {
	engine := NewEngine(nil, nil, nil, nil)
	dep := &Deployment{Status: "ROLLING_OUT"}
	healthy, reason := engine.EvaluateHealthGate(context.Background(), dep)
	assert.True(t, healthy)
	assert.Equal(t, "no health gate configured", reason)
}

func TestEngine_HealthGate_TelemetryError(t *testing.T) {
	tel := &mockTelemetry{err: fmt.Errorf("connection refused")}
	engine := NewEngine(nil, tel, nil, nil)
	dep := &Deployment{Status: "ROLLING_OUT", HealthGate: &HealthGate{MaxErrorRate: 0.05, MinEvents: 10}}
	healthy, reason := engine.EvaluateHealthGate(context.Background(), dep)
	assert.True(t, healthy) // Fail open
	assert.Contains(t, reason, "passing by default")
}

func TestEngine_GetHealthStatus(t *testing.T) {
	repo := newMockRepo()
	tel := &mockTelemetry{stats: &DeploymentStats{TotalEvents: 500, ErrorRate: 0.02}}
	engine := NewEngine(repo, tel, nil, nil)

	gate := &HealthGate{MaxErrorRate: 0.05, MinEvents: 100}
	dep := createPendingDep(repo, "CANARY", quickStages(), gate)
	_, _ = engine.StartDeployment(context.Background(), dep.ID)

	status, err := engine.GetHealthStatus(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, true, status["healthy"])
	assert.Equal(t, true, status["can_progress"])
}

func TestEngine_EffectiveCanaryPercent(t *testing.T) {
	d := &Deployment{Strategy: "ALL_AT_ONCE"}
	assert.Equal(t, 100, d.EffectiveCanaryPercent())

	d = &Deployment{Strategy: "CANARY", Status: "COMPLETE"}
	assert.Equal(t, 100, d.EffectiveCanaryPercent())

	d = &Deployment{Strategy: "CANARY", Status: "ROLLING_OUT", CurrentStage: 1,
		CanaryStages: []CanaryStage{{Percentage: 1}, {Percentage: 10}, {Percentage: 50}, {Percentage: 100}}}
	assert.Equal(t, 10, d.EffectiveCanaryPercent())
}

// Rolling back a COMPLETE deployment creates a COMPLETE revert deployment of
// the previous version, so devices get an explicit LOAD/UPDATE back to it.
func TestEngine_RollbackFromComplete_CreatesRevert(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "ALL_AT_ONCE", nil, nil)
	prev := uuid.New()
	dep.PreviousVersionID = &prev

	got, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	require.Equal(t, "COMPLETE", got.Status)

	got, err = engine.RollbackDeployment(context.Background(), dep.ID, "regression found after release")
	require.NoError(t, err)
	assert.Equal(t, "ROLLED_BACK", got.Status)

	var revert *Deployment
	for _, d := range repo.deployments {
		if d.ID != dep.ID {
			revert = d
		}
	}
	require.NotNil(t, revert, "a revert deployment is created")
	assert.Equal(t, prev, revert.ModelVersionID)
	assert.Equal(t, "COMPLETE", revert.Status)
	assert.Equal(t, dep.TargetSelector, revert.TargetSelector)

	_, err = engine.RollbackDeployment(context.Background(), dep.ID, "again")
	assert.ErrorIs(t, err, ErrInvalidTransition, "ROLLED_BACK is terminal")
}

func TestEngine_RollbackWithoutPreviousVersion(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, nil, nil)
	dep := createPendingDep(repo, "ALL_AT_ONCE", nil, nil)
	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	_, err = engine.RollbackDeployment(context.Background(), dep.ID, "no previous")
	require.NoError(t, err)
	assert.Len(t, repo.deployments, 1, "no revert without a previous version")
}

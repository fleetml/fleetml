package deploy

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/rollout"
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
	stats      *DeploymentStats
	err        error
	prevP95    float64
	prevEvents int64
	gotDevices []uuid.UUID
	gotSince   time.Time
}

func (m *mockTelemetry) CohortStats(_ context.Context, _ uuid.UUID, deviceIDs []uuid.UUID, since time.Time) (*DeploymentStats, error) {
	m.gotDevices, m.gotSince = deviceIDs, since
	if m.err != nil {
		return nil, m.err
	}
	return m.stats, nil
}

func (m *mockTelemetry) VersionP95(context.Context, uuid.UUID, time.Time) (p95 float64, events int64, err error) {
	return m.prevP95, m.prevEvents, nil
}

// mockFleet is a fixed set of targeted devices.
type mockFleet struct {
	ids         []uuid.UUID
	unavailable int
}

func newMockFleet(n int) *mockFleet {
	f := &mockFleet{}
	for i := 0; i < n; i++ {
		f.ids = append(f.ids, uuid.New())
	}
	return f
}

func (f *mockFleet) CountUnavailable(context.Context, []uuid.UUID, uuid.UUID, time.Time) (int, error) {
	return f.unavailable, nil
}

func (f *mockFleet) ListMatchingDevices(context.Context, map[string]string) ([]uuid.UUID, error) {
	return f.ids, nil
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
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
	dep := createPendingDep(repo, "ALL_AT_ONCE", nil, nil)

	result, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "COMPLETE", result.Status)
	assert.NotNil(t, result.CompletedAt)
}

func TestEngine_Canary_FullProgression(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
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
	engine := NewEngine(repo, tel, newMockFleet(50), nil)

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
	engine := NewEngine(repo, tel, newMockFleet(50), nil)

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
	engine := NewEngine(repo, tel, newMockFleet(50), nil)

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
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)

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
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
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
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
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
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)

	_, _ = engine.StartDeployment(context.Background(), dep.ID)
	_, _ = engine.PauseDeployment(context.Background(), dep.ID)

	result, err := engine.RollbackDeployment(context.Background(), dep.ID, "decided to rollback")
	require.NoError(t, err)
	assert.Equal(t, "ROLLED_BACK", result.Status)
}

func TestEngine_InvalidTransition_StartComplete(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
	dep := createPendingDep(repo, "ALL_AT_ONCE", nil, nil)

	_, _ = engine.StartDeployment(context.Background(), dep.ID) // COMPLETE

	_, err := engine.StartDeployment(context.Background(), dep.ID)
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestEngine_InvalidTransition_PausePending(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)

	_, err := engine.PauseDeployment(context.Background(), dep.ID) // Can't pause PENDING
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestEngine_InvalidTransition_RollbackPending(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)

	_, err := engine.RollbackDeployment(context.Background(), dep.ID, "test")
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestEngine_ProgressComplete_NoOp(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
	dep := createPendingDep(repo, "ALL_AT_ONCE", nil, nil)

	_, _ = engine.StartDeployment(context.Background(), dep.ID)

	result, err := engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "COMPLETE", result.Status) // No-op
}

func createRollingDep(repo *mockRepo, rc *RollingConfig) *Deployment {
	d := createPendingDep(repo, "ROLLING", nil, nil)
	d.RollingConfig = rc
	return d
}

// exposedCount counts the fleet devices a deployment currently delivers to.
func exposedCount(d *Deployment, fleet *mockFleet) int {
	n := 0
	for _, id := range fleet.ids {
		if d.Exposes(id) {
			n++
		}
	}
	return n
}

// 20 devices, batch_size 5: successive stages expose exactly 5, 10, 15, 20.
func TestEngine_Rolling_CountBatches(t *testing.T) {
	repo := newMockRepo()
	fleet := newMockFleet(20)
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, fleet, nil)
	dep := createRollingDep(repo, &RollingConfig{BatchSize: 5, BatchInterval: "0s"})

	got, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "ROLLING_OUT", got.Status)
	var seen []int
	seen = append(seen, exposedCount(got, fleet))
	for i := 0; i < 3; i++ {
		got, err = engine.ProgressDeployment(context.Background(), dep.ID)
		require.NoError(t, err)
		seen = append(seen, exposedCount(got, fleet))
	}
	assert.Equal(t, []int{5, 10, 15, 20}, seen)
	assert.Equal(t, "ROLLING_OUT", got.Status, "the last batch still gets its interval")
	got, err = engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "COMPLETE", got.Status)
}

func TestEngine_Rolling_PercentBatchesAndInterval(t *testing.T) {
	repo := newMockRepo()
	fleet := newMockFleet(1000)
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, fleet, nil)
	dep := createRollingDep(repo, &RollingConfig{BatchSizePercent: 30, BatchInterval: "1h"})

	got, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.InDelta(t, 300, exposedCount(got, fleet), 60)
	assert.Equal(t, 30, got.EffectiveCanaryPercent())

	got, err = engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.CurrentStage, "batch_interval not elapsed: no advance")

	past := time.Now().Add(-2 * time.Hour)
	repo.deployments[dep.ID].StageStartedAt = &past
	got, err = engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.CurrentStage)
	assert.Equal(t, 60, got.EffectiveCanaryPercent())
}

func TestEngine_Rolling_MaxUnavailableHolds(t *testing.T) {
	repo := newMockRepo()
	fleet := newMockFleet(20)
	fleet.unavailable = 3
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, fleet, nil)
	dep := createRollingDep(repo, &RollingConfig{BatchSize: 5, BatchInterval: "0s", MaxUnavailable: 2})
	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	got, err := engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.CurrentStage, "3 unavailable > max 2: held")

	fleet.unavailable = 2
	got, err = engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.CurrentStage)
}

func TestEngine_Rolling_GateFailureRollsBack(t *testing.T) {
	repo := newMockRepo()
	tel := &mockTelemetry{stats: &DeploymentStats{}}
	engine := NewEngine(repo, tel, newMockFleet(20), nil)
	dep := createRollingDep(repo, &RollingConfig{BatchSize: 5, BatchInterval: "0s"})
	dep.HealthGate = DefaultHealthGate()
	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	got, err := engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	require.Equal(t, 2, got.CurrentStage)

	tel.stats = &DeploymentStats{TotalEvents: 40, ErrorCount: 20, ErrorRate: 0.5}
	got, err = engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "ROLLED_BACK", got.Status)
	assert.Equal(t, 2, got.CurrentStage, "stopped in stage 2")
}

func TestEngine_HealthGate_DefaultWhenNoGate(t *testing.T) {
	tel := &mockTelemetry{stats: &DeploymentStats{TotalEvents: 40, ErrorCount: 40, ErrorRate: 1}}
	engine := NewEngine(nil, tel, newMockFleet(10), nil)
	dep := &Deployment{ID: uuid.New(), Strategy: "CANARY", Status: "ROLLING_OUT"}
	res := engine.EvaluateHealthGate(context.Background(), dep)
	assert.False(t, res.Healthy, "a deployment without a gate still uses the default one")
	assert.Contains(t, res.Reason, "error rate")

	tel.stats = &DeploymentStats{TotalEvents: 19, ErrorCount: 19, ErrorRate: 1}
	res = engine.EvaluateHealthGate(context.Background(), dep)
	assert.True(t, res.Healthy, "below the default 20-event minimum the gate does not judge")
}

func TestEngine_HealthGate_TelemetryErrorHolds(t *testing.T) {
	repo := newMockRepo()
	tel := &mockTelemetry{err: fmt.Errorf("connection refused")}
	engine := NewEngine(repo, tel, newMockFleet(10), nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)
	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	res := engine.EvaluateHealthGate(context.Background(), dep)
	assert.True(t, res.Hold, "fails closed")
	assert.False(t, res.Healthy)

	got, err := engine.ProgressDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, "ROLLING_OUT", got.Status, "neither advances nor rolls back")
	assert.Equal(t, 0, got.CurrentStage)
}

// Stats come only from the devices the current stage exposes, since the stage started.
func TestEngine_HealthGate_ScopedToCohort(t *testing.T) {
	repo := newMockRepo()
	fleet := newMockFleet(400)
	tel := &mockTelemetry{stats: &DeploymentStats{}}
	engine := NewEngine(repo, tel, fleet, nil)
	stages := []CanaryStage{{Percentage: 10, MinDuration: "1h", Status: "PENDING"}, {Percentage: 100, MinDuration: "0s", Status: "PENDING"}}
	dep := createPendingDep(repo, "CANARY", stages, nil)
	started, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	engine.EvaluateHealthGate(context.Background(), started)
	require.NotEmpty(t, tel.gotDevices)
	assert.Less(t, len(tel.gotDevices), 80, "about 10% of 400 devices")
	for _, id := range tel.gotDevices {
		assert.True(t, rollout.InCanary(id, dep.ID, 10), "only canary devices are counted")
	}
	assert.Equal(t, *started.CanaryStages[0].StartedAt, tel.gotSince, "window starts with the stage")
}

func TestEngine_HealthGate_LatencyRegression(t *testing.T) {
	prev := uuid.New()
	tel := &mockTelemetry{stats: &DeploymentStats{TotalEvents: 100, P95LatencyMs: 130}, prevP95: 100, prevEvents: 500}
	engine := NewEngine(nil, tel, newMockFleet(10), nil)
	dep := &Deployment{ID: uuid.New(), Strategy: "CANARY", Status: "ROLLING_OUT", PreviousVersionID: &prev,
		HealthGate: &HealthGate{MaxErrorRate: 0.05, MaxLatencyRegression: 0.2, MinEvents: 20, OnFailure: "ROLLBACK"}}

	res := engine.EvaluateHealthGate(context.Background(), dep)
	assert.False(t, res.Healthy)
	assert.Contains(t, res.Reason, "p95 latency 130ms")

	tel.stats.P95LatencyMs = 115
	assert.True(t, engine.EvaluateHealthGate(context.Background(), dep).Healthy, "+15% is within +20%")

	tel.stats.P95LatencyMs = 500
	tel.prevEvents = 5
	assert.True(t, engine.EvaluateHealthGate(context.Background(), dep).Healthy, "no judgement without a baseline")
}

func TestEngine_GetHealthStatus(t *testing.T) {
	repo := newMockRepo()
	tel := &mockTelemetry{stats: &DeploymentStats{TotalEvents: 500, ErrorRate: 0.02}}
	engine := NewEngine(repo, tel, newMockFleet(50), nil)

	gate := &HealthGate{MaxErrorRate: 0.05, MinEvents: 100}
	dep := createPendingDep(repo, "CANARY", quickStages(), gate)
	_, _ = engine.StartDeployment(context.Background(), dep.ID)

	status, err := engine.GetHealthStatus(context.Background(), dep.ID)
	require.NoError(t, err)
	assert.Equal(t, true, status["healthy"])
	assert.Equal(t, false, status["hold"])
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
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
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
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(50), nil)
	dep := createPendingDep(repo, "ALL_AT_ONCE", nil, nil)
	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)
	_, err = engine.RollbackDeployment(context.Background(), dep.ID, "no previous")
	require.NoError(t, err)
	assert.Len(t, repo.deployments, 1, "no revert without a previous version")
}

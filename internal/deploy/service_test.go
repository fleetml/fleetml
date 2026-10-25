package deploy

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRepo and newMockRepo are defined in engine_test.go

func TestCreateDeployment(t *testing.T) {
	svc := NewService(newMockRepo(), nil)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		d, err := svc.Create(ctx, CreateInput{
			ProjectID:      uuid.New(),
			ModelVersionID: uuid.New(),
			Strategy:       "ALL_AT_ONCE",
		})
		require.NoError(t, err)
		assert.Equal(t, "PENDING", d.Status)
		assert.Equal(t, "ALL_AT_ONCE", d.Strategy)
	})

	t.Run("default strategy", func(t *testing.T) {
		d, err := svc.Create(ctx, CreateInput{
			ProjectID:      uuid.New(),
			ModelVersionID: uuid.New(),
		})
		require.NoError(t, err)
		assert.Equal(t, "ALL_AT_ONCE", d.Strategy)
	})

	t.Run("invalid strategy", func(t *testing.T) {
		_, err := svc.Create(ctx, CreateInput{
			ProjectID:      uuid.New(),
			ModelVersionID: uuid.New(),
			Strategy:       "INVALID",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("canary default stages", func(t *testing.T) {
		d, err := svc.Create(ctx, CreateInput{
			ProjectID:      uuid.New(),
			ModelVersionID: uuid.New(),
			Strategy:       "CANARY",
		})
		require.NoError(t, err)
		assert.Equal(t, "CANARY", d.Strategy)
		assert.Len(t, d.CanaryStages, 4)
		assert.Equal(t, 1, d.CanaryStages[0].Percentage)
		assert.Equal(t, 100, d.CanaryStages[3].Percentage)
	})

	t.Run("canary custom stages", func(t *testing.T) {
		d, err := svc.Create(ctx, CreateInput{
			ProjectID:      uuid.New(),
			ModelVersionID: uuid.New(),
			Strategy:       "CANARY",
			CanaryStages: []CanaryStageInput{
				{Percentage: 5, MinDuration: "10m"},
				{Percentage: 50, MinDuration: "30m"},
				{Percentage: 100, MinDuration: "0s"},
			},
		})
		require.NoError(t, err)
		assert.Len(t, d.CanaryStages, 3)
	})

	t.Run("canary stages must end at 100", func(t *testing.T) {
		_, err := svc.Create(ctx, CreateInput{
			ProjectID:      uuid.New(),
			ModelVersionID: uuid.New(),
			Strategy:       "CANARY",
			CanaryStages:   []CanaryStageInput{{Percentage: 50, MinDuration: "5m"}},
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("canary stages must increase", func(t *testing.T) {
		_, err := svc.Create(ctx, CreateInput{
			ProjectID:      uuid.New(),
			ModelVersionID: uuid.New(),
			Strategy:       "CANARY",
			CanaryStages: []CanaryStageInput{
				{Percentage: 50, MinDuration: "5m"},
				{Percentage: 30, MinDuration: "5m"},
				{Percentage: 100, MinDuration: "0s"},
			},
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("rolling default config", func(t *testing.T) {
		d, err := svc.Create(ctx, CreateInput{
			ProjectID:      uuid.New(),
			ModelVersionID: uuid.New(),
			Strategy:       "ROLLING",
		})
		require.NoError(t, err)
		assert.NotNil(t, d.RollingConfig)
		assert.Equal(t, 10, d.RollingConfig.BatchSizePercent)
		assert.Equal(t, "5m", d.RollingConfig.BatchInterval)
	})

	t.Run("rolling invalid batch size", func(t *testing.T) {
		_, err := svc.Create(ctx, CreateInput{
			ProjectID:      uuid.New(),
			ModelVersionID: uuid.New(),
			Strategy:       "ROLLING",
			RollingConfig:  &RollingConfig{BatchSize: 0},
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("health gate validation", func(t *testing.T) {
		_, err := svc.Create(ctx, CreateInput{
			ProjectID:      uuid.New(),
			ModelVersionID: uuid.New(),
			HealthGate:     &HealthGate{MaxErrorRate: 2.0},
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})
}

// progress moves only a ROLLING_OUT deployment; any other status is refused,
// as pause and cancel refuse theirs.
func TestProgressDeployment_OnlyRollingOut(t *testing.T) {
	svc := newServiceWithEngine()
	ctx := context.Background()
	d, err := svc.Create(ctx, CreateInput{ProjectID: uuid.New(), ModelVersionID: uuid.New(), Strategy: "CANARY"})
	require.NoError(t, err)

	_, err = svc.ProgressDeployment(ctx, d.ID)
	assert.ErrorIs(t, err, ErrInvalidTransition, "PENDING")
	assert.ErrorContains(t, err, "cannot progress a PENDING deployment")

	_, err = svc.UpdateStatus(ctx, d.ID, "ROLLING_OUT", "")
	require.NoError(t, err)
	_, err = svc.ProgressDeployment(ctx, d.ID)
	require.NoError(t, err)

	_, err = svc.UpdateStatus(ctx, d.ID, "ROLLED_BACK", "test")
	require.NoError(t, err)
	_, err = svc.ProgressDeployment(ctx, d.ID)
	assert.ErrorIs(t, err, ErrInvalidTransition, "ROLLED_BACK")
}

func newServiceWithEngine() *Service {
	repo := newMockRepo()
	svc := NewService(repo, nil)
	svc.SetEngine(NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(10), nil))
	return svc
}

// PATCH /status goes through the state machine: legal moves have their side
// effects, illegal ones are rejected instead of being written blindly.
func TestUpdateDeploymentStatus(t *testing.T) {
	svc := newServiceWithEngine()
	ctx := context.Background()

	d, err := svc.Create(ctx, CreateInput{ProjectID: uuid.New(), ModelVersionID: uuid.New(), Strategy: "CANARY"})
	require.NoError(t, err)

	_, err = svc.UpdateStatus(ctx, d.ID, "INVALID", "")
	assert.ErrorIs(t, err, ErrInvalidInput)
	_, err = svc.UpdateStatus(ctx, d.ID, "COMPLETE", "")
	assert.ErrorIs(t, err, ErrInvalidTransition, "PENDING cannot jump to COMPLETE")
	_, err = svc.UpdateStatus(ctx, d.ID, "FAILED", "")
	assert.ErrorIs(t, err, ErrInvalidTransition, "FAILED is not settable")

	got, err := svc.UpdateStatus(ctx, d.ID, "ROLLING_OUT", "")
	require.NoError(t, err)
	assert.Equal(t, "ROLLING_OUT", got.Status)
	assert.Equal(t, "ACTIVE", got.CanaryStages[0].Status, "starting via PATCH has the same effect as /start")

	got, err = svc.UpdateStatus(ctx, d.ID, "PAUSED", "")
	require.NoError(t, err)
	assert.Equal(t, "PAUSED", got.Status)
	got, err = svc.UpdateStatus(ctx, d.ID, "ROLLING_OUT", "")
	require.NoError(t, err)
	assert.Equal(t, "ROLLING_OUT", got.Status)

	got, err = svc.UpdateStatus(ctx, d.ID, "COMPLETE", "")
	require.NoError(t, err)
	assert.Equal(t, "COMPLETE", got.Status)
	_, err = svc.UpdateStatus(ctx, d.ID, "PENDING", "")
	assert.ErrorIs(t, err, ErrInvalidTransition)
	_, err = svc.UpdateStatus(ctx, d.ID, "CANCELLED", "")
	assert.ErrorIs(t, err, ErrInvalidTransition, "a COMPLETE deployment is rolled back, not cancelled")

	got, err = svc.UpdateStatus(ctx, d.ID, "ROLLED_BACK", "bad metrics")
	require.NoError(t, err)
	assert.Equal(t, "ROLLED_BACK", got.Status)
	assert.Equal(t, "bad metrics", got.RollbackReason)
	_, err = svc.UpdateStatus(ctx, d.ID, "ROLLING_OUT", "")
	assert.ErrorIs(t, err, ErrInvalidTransition, "ROLLED_BACK is terminal")
}

func TestCancelDeployment(t *testing.T) {
	svc := newServiceWithEngine()
	ctx := context.Background()
	for _, start := range []bool{false, true} {
		d, err := svc.Create(ctx, CreateInput{ProjectID: uuid.New(), ModelVersionID: uuid.New(), Strategy: "CANARY"})
		require.NoError(t, err)
		if start {
			_, err = svc.StartDeployment(ctx, d.ID)
			require.NoError(t, err)
		}
		got, err := svc.CancelDeployment(ctx, d.ID)
		require.NoError(t, err)
		assert.Equal(t, "CANCELLED", got.Status)
		assert.NotNil(t, got.CompletedAt)
		_, err = svc.CancelDeployment(ctx, d.ID)
		assert.ErrorIs(t, err, ErrInvalidTransition)
	}
}

func TestDeleteDeployment(t *testing.T) {
	svc := NewService(newMockRepo(), nil)
	ctx := context.Background()

	d, err := svc.Create(ctx, CreateInput{
		ProjectID: uuid.New(), ModelVersionID: uuid.New(),
	})
	require.NoError(t, err)

	err = svc.Delete(ctx, d.ID)
	assert.NoError(t, err)

	err = svc.Delete(ctx, d.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

type fakeVersions map[uuid.UUID]int

func (f fakeVersions) ReadyVariantCount(_ context.Context, id uuid.UUID) (int, error) {
	return f[id], nil
}

func (f fakeVersions) CurrentVersion(context.Context, uuid.UUID, uuid.UUID) (*uuid.UUID, error) {
	return nil, nil
}

// currentVersion reports a fixed current version for every model.
type currentVersion struct{ id uuid.UUID }

func (c currentVersion) ReadyVariantCount(context.Context, uuid.UUID) (int, error) { return 1, nil }
func (c currentVersion) CurrentVersion(context.Context, uuid.UUID, uuid.UUID) (*uuid.UUID, error) {
	return &c.id, nil
}

func TestCreate_RecordsPreviousVersion(t *testing.T) {
	v1 := uuid.New()
	svc := NewService(newMockRepo(), nil)
	svc.SetVersionChecker(currentVersion{id: v1})
	d, err := svc.Create(context.Background(), CreateInput{ProjectID: uuid.New(), ModelVersionID: uuid.New()})
	require.NoError(t, err)
	require.NotNil(t, d.PreviousVersionID)
	assert.Equal(t, v1, *d.PreviousVersionID)
}

func TestCreate_RequiresReadyVariant(t *testing.T) {
	ready, unverified := uuid.New(), uuid.New()
	svc := NewService(newMockRepo(), nil)
	svc.SetVersionChecker(fakeVersions{ready: 1, unverified: 0})

	_, err := svc.Create(context.Background(), CreateInput{ProjectID: uuid.New(), ModelVersionID: unverified})
	assert.ErrorIs(t, err, ErrNotDeployable)

	d, err := svc.Create(context.Background(), CreateInput{ProjectID: uuid.New(), ModelVersionID: ready})
	require.NoError(t, err)
	assert.Equal(t, "PENDING", d.Status)
}

// Reading one deployment tells how many targeted devices report its version
// loaded: COMPLETE alone does not mean the model reached them.
func TestGet_DeviceProgress(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil)
	ctx := context.Background()
	d, err := svc.Create(ctx, CreateInput{ProjectID: uuid.New(), ModelVersionID: uuid.New()})
	require.NoError(t, err)

	got, err := svc.Get(ctx, d.ID)
	require.NoError(t, err)
	assert.Nil(t, got.Devices, "no engine, no counts")

	fleet := newMockFleet(3)
	fleet.loaded = 1
	svc.SetEngine(NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, fleet, nil))
	got, err = svc.Get(ctx, d.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Devices)
	assert.Equal(t, DeviceProgress{Targeted: 3, Loaded: 1}, *got.Devices)
}

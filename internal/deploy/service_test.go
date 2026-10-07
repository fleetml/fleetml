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
		assert.Equal(t, 10, d.RollingConfig.BatchSize)
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

func TestUpdateDeploymentStatus(t *testing.T) {
	svc := NewService(newMockRepo(), nil)
	ctx := context.Background()

	d, err := svc.Create(ctx, CreateInput{
		ProjectID: uuid.New(), ModelVersionID: uuid.New(),
	})
	require.NoError(t, err)

	err = svc.UpdateStatus(ctx, d.ID, "COMPLETE")
	assert.NoError(t, err)

	err = svc.UpdateStatus(ctx, d.ID, "INVALID")
	assert.ErrorIs(t, err, ErrInvalidInput)
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

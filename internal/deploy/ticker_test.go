package deploy

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTicker_ProgressesRollingOut(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(10), nil)
	dep := createPendingDep(repo, "CANARY", quickStages(), nil)
	_, err := engine.StartDeployment(context.Background(), dep.ID)
	require.NoError(t, err)

	ticker := NewTicker(engine, repo, time.Minute, nil)
	ticker.tick(context.Background())

	updated, _ := repo.GetByID(context.Background(), dep.ID)
	// Should have progressed (stage 0 had 0s min_duration)
	assert.Equal(t, 1, updated.CurrentStage)
}

func TestTicker_SkipsCompleted(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(10), nil)

	dep := &Deployment{
		ProjectID: uuid.New(), ModelVersionID: uuid.New(),
		Strategy: "ALL_AT_ONCE", Status: "COMPLETE",
		TargetSelector: map[string]string{}, CreatedBy: "test",
	}
	_ = repo.Create(context.Background(), dep)

	ticker := NewTicker(engine, repo, time.Minute, nil)
	ticker.tick(context.Background()) // Should not panic or error
	// COMPLETE deployment should remain COMPLETE
	updated, _ := repo.GetByID(context.Background(), dep.ID)
	assert.Equal(t, "COMPLETE", updated.Status)
}

func TestTicker_EmptyList(t *testing.T) {
	repo := newMockRepo()
	engine := NewEngine(repo, &mockTelemetry{stats: &DeploymentStats{}}, newMockFleet(10), nil)
	ticker := NewTicker(engine, repo, time.Minute, nil)
	// Should not panic with empty deployment list
	ticker.tick(context.Background())
}

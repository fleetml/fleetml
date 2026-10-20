package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/deploy"
)

// --- Mock deploy repo ---

type mockDeployRepo struct {
	deployments map[uuid.UUID]*deploy.Deployment
}

func newMockDeployRepo() *mockDeployRepo {
	return &mockDeployRepo{deployments: make(map[uuid.UUID]*deploy.Deployment)}
}

func (m *mockDeployRepo) Create(_ context.Context, d *deploy.Deployment) error {
	d.ID = uuid.New()
	d.CreatedAt = time.Now()
	d.UpdatedAt = time.Now()
	m.deployments[d.ID] = d
	return nil
}

func (m *mockDeployRepo) GetByID(_ context.Context, id uuid.UUID) (*deploy.Deployment, error) {
	d, ok := m.deployments[id]
	if !ok {
		return nil, fmt.Errorf("deployment.GetByID: %w", deploy.ErrNotFound)
	}
	return d, nil
}

func (m *mockDeployRepo) List(_ context.Context, projectID *uuid.UUID, limit, offset int) ([]deploy.Deployment, int, error) {
	var all []deploy.Deployment
	for _, d := range m.deployments {
		if projectID == nil || d.ProjectID == *projectID {
			all = append(all, *d)
		}
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

func (m *mockDeployRepo) ListByStatus(_ context.Context, statuses []string) ([]deploy.Deployment, error) {
	return nil, nil
}

func (m *mockDeployRepo) Update(_ context.Context, d *deploy.Deployment) error {
	if _, ok := m.deployments[d.ID]; !ok {
		return deploy.ErrNotFound
	}
	m.deployments[d.ID] = d
	return nil
}

func (m *mockDeployRepo) UpdateStatus(_ context.Context, id uuid.UUID, status string) error {
	d, ok := m.deployments[id]
	if !ok {
		return deploy.ErrNotFound
	}
	d.Status = status
	return nil
}

func (m *mockDeployRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.deployments[id]; !ok {
		return fmt.Errorf("deployment.Delete: %w", deploy.ErrNotFound)
	}
	delete(m.deployments, id)
	return nil
}

type mockDeployTelemetry struct{}

func (m *mockDeployTelemetry) CohortStats(context.Context, uuid.UUID, []uuid.UUID, time.Time) (*deploy.DeploymentStats, error) {
	return &deploy.DeploymentStats{TotalEvents: 500, ErrorRate: 0.01}, nil
}

func (m *mockDeployTelemetry) VersionP95(context.Context, uuid.UUID, time.Time) (p95 float64, events int64, err error) {
	return 0, 0, nil
}

// mockDeployFleet is a fixed set of targeted devices.
type mockDeployFleet struct{}

func (mockDeployFleet) CountUnavailable(context.Context, []uuid.UUID, uuid.UUID, time.Time) (int, error) {
	return 0, nil
}

func (mockDeployFleet) ListMatchingDevices(context.Context, map[string]string) ([]uuid.UUID, error) {
	return []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}, nil
}

func setupDeployTestRouter() (*gin.Engine, *deploy.Service) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	repo := newMockDeployRepo()
	svc := deploy.NewService(repo, logger)
	engine := deploy.NewEngine(repo, &mockDeployTelemetry{}, mockDeployFleet{}, nil)
	svc.SetEngine(engine)

	r := gin.New()
	api := r.Group("/api/v1")
	NewDeploymentHandler(svc, logger).RegisterRoutes(api)
	return r, svc
}

func TestDeployCreate_CanaryStages(t *testing.T) {
	r, _ := setupDeployTestRouter()
	w := doJSON(r, "POST", "/api/v1/deployments", map[string]interface{}{
		"project_id":       uuid.New().String(),
		"model_version_id": uuid.New().String(),
		"strategy":         "CANARY",
		"canary_stages": []map[string]interface{}{
			{"percentage": 10, "min_duration": "5m"},
			{"percentage": 50, "min_duration": "15m"},
			{"percentage": 100, "min_duration": "0s"},
		},
	})
	assert.Equal(t, http.StatusCreated, w.Code)
	body := parseBody(t, w)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "CANARY", data["strategy"])
	stages := data["canary_stages"].([]interface{})
	assert.Len(t, stages, 3)
}

func TestDeployStartAndRollback(t *testing.T) {
	r, svc := setupDeployTestRouter()
	ctx := context.Background()

	dep, err := svc.Create(ctx, deploy.CreateInput{
		ProjectID: uuid.New(), ModelVersionID: uuid.New(), Strategy: "CANARY",
	})
	require.NoError(t, err)

	// Start
	w := doJSON(r, "POST", fmt.Sprintf("/api/v1/deployments/%s/start", dep.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "ROLLING_OUT", body["data"].(map[string]interface{})["status"])

	// Rollback
	w = doJSON(r, "POST", fmt.Sprintf("/api/v1/deployments/%s/rollback", dep.ID), map[string]string{"reason": "test"})
	assert.Equal(t, http.StatusOK, w.Code)
	body = parseBody(t, w)
	assert.Equal(t, "ROLLED_BACK", body["data"].(map[string]interface{})["status"])
}

func TestDeployHealth(t *testing.T) {
	r, svc := setupDeployTestRouter()
	ctx := context.Background()

	dep, err := svc.Create(ctx, deploy.CreateInput{
		ProjectID: uuid.New(), ModelVersionID: uuid.New(), Strategy: "CANARY",
		HealthGate: &deploy.HealthGate{MaxErrorRate: 0.05, MinEvents: 10},
	})
	require.NoError(t, err)
	_, _ = svc.StartDeployment(ctx, dep.ID)

	w := doJSON(r, "GET", fmt.Sprintf("/api/v1/deployments/%s/health", dep.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, true, data["healthy"])
}

func TestDeployInvalidTransition(t *testing.T) {
	r, svc := setupDeployTestRouter()
	ctx := context.Background()

	dep, _ := svc.Create(ctx, deploy.CreateInput{
		ProjectID: uuid.New(), ModelVersionID: uuid.New(),
	})
	// Start ALL_AT_ONCE → COMPLETE
	_, _ = svc.StartDeployment(ctx, dep.ID)

	// Try to start again → should fail
	w := doJSON(r, "POST", fmt.Sprintf("/api/v1/deployments/%s/start", dep.ID), nil)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestDeployPauseResume(t *testing.T) {
	r, svc := setupDeployTestRouter()
	ctx := context.Background()

	dep, _ := svc.Create(ctx, deploy.CreateInput{
		ProjectID: uuid.New(), ModelVersionID: uuid.New(), Strategy: "CANARY",
	})
	_, _ = svc.StartDeployment(ctx, dep.ID)

	w := doJSON(r, "POST", fmt.Sprintf("/api/v1/deployments/%s/pause", dep.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)

	w = doJSON(r, "POST", fmt.Sprintf("/api/v1/deployments/%s/resume", dep.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

// project_id lists only that project's deployments, like the device list;
// a value that is not a UUID is refused rather than ignored.
func TestDeployListByProject(t *testing.T) {
	r, svc := setupDeployTestRouter()
	ctx := context.Background()
	p1, p2 := uuid.New(), uuid.New()
	for _, p := range []uuid.UUID{p1, p1, p2} {
		_, err := svc.Create(ctx, deploy.CreateInput{ProjectID: p, ModelVersionID: uuid.New(), Strategy: "ALL_AT_ONCE"})
		require.NoError(t, err)
	}
	total := func(query string) float64 {
		t.Helper()
		w := doJSON(r, "GET", "/api/v1/deployments"+query, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return parseBody(t, w)["pagination"].(map[string]interface{})["total"].(float64)
	}
	assert.Equal(t, float64(3), total(""))
	assert.Equal(t, float64(2), total("?project_id="+p1.String()))
	assert.Equal(t, float64(1), total("?project_id="+p2.String()))
	assert.Equal(t, float64(0), total("?project_id="+uuid.New().String()))
	w := doJSON(r, "GET", "/api/v1/deployments?project_id=not-a-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

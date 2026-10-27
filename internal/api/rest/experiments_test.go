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

	"github.com/fleetml/fleetml/internal/experiment"
)

// --- Mock experiment repo ---

type mockExpRepo struct {
	experiments map[uuid.UUID]*experiment.Experiment
	names       map[string]uuid.UUID
}

func newMockExpRepo() *mockExpRepo {
	return &mockExpRepo{experiments: make(map[uuid.UUID]*experiment.Experiment), names: make(map[string]uuid.UUID)}
}

func (m *mockExpRepo) Create(_ context.Context, e *experiment.Experiment) error {
	key := e.ProjectID.String() + ":" + e.Name
	if _, exists := m.names[key]; exists {
		return fmt.Errorf("experiment.Create: %w: name %q", experiment.ErrConflict, e.Name)
	}
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	e.UpdatedAt = time.Now()
	m.experiments[e.ID] = e
	m.names[key] = e.ID
	return nil
}

func (m *mockExpRepo) GetByID(_ context.Context, id uuid.UUID) (*experiment.Experiment, error) {
	e, ok := m.experiments[id]
	if !ok {
		return nil, fmt.Errorf("experiment.GetByID: %w", experiment.ErrNotFound)
	}
	return e, nil
}

func (m *mockExpRepo) List(_ context.Context, _ *uuid.UUID, limit, offset int) ([]experiment.Experiment, int, error) {
	var all []experiment.Experiment
	for _, e := range m.experiments {
		all = append(all, *e)
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

func (m *mockExpRepo) Update(_ context.Context, e *experiment.Experiment) error {
	if _, ok := m.experiments[e.ID]; !ok {
		return experiment.ErrNotFound
	}
	m.experiments[e.ID] = e
	return nil
}

func (m *mockExpRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.experiments[id]; !ok {
		return fmt.Errorf("experiment.Delete: %w", experiment.ErrNotFound)
	}
	delete(m.experiments, id)
	return nil
}

func (m *mockExpRepo) ListByStatus(_ context.Context, _ []string) ([]experiment.Experiment, error) {
	return nil, nil
}

func (m *mockExpRepo) AssignCohort(_ context.Context, _ *experiment.CohortAssignment) error {
	return nil
}

func (m *mockExpRepo) GetCohortAssignment(_ context.Context, _, _ uuid.UUID) (*experiment.CohortAssignment, error) {
	return nil, nil
}

func (m *mockExpRepo) ListCohortAssignments(_ context.Context, _ uuid.UUID) ([]experiment.CohortAssignment, error) {
	return nil, nil
}

func (m *mockExpRepo) GetCohortStats(_ context.Context, _, _ uuid.UUID, _ time.Time, _ string) (*experiment.CohortStats, error) {
	return &experiment.CohortStats{SampleSize: 0}, nil
}

func setupExpTestRouter() (*gin.Engine, *experiment.Service) {
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := experiment.NewService(newMockExpRepo(), logger)
	r := gin.New()
	api := r.Group("/api/v1")
	NewExperimentHandler(svc, logger).RegisterRoutes(api)
	return r, svc
}

func TestExpCreate_Valid(t *testing.T) {
	r, _ := setupExpTestRouter()
	w := doJSON(r, "POST", "/api/v1/experiments", map[string]interface{}{
		"project_id": uuid.New().String(), "name": "test",
		"control_version_id": uuid.New().String(), "treatment_version_id": uuid.New().String(),
	})
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestExpStart(t *testing.T) {
	r, svc := setupExpTestRouter()
	exp, _ := svc.Create(context.Background(), experiment.CreateInput{
		ProjectID: uuid.New(), Name: "start-test",
		ControlVersionID: uuid.New(), TreatmentVersionID: uuid.New(),
	})
	w := doJSON(r, "POST", fmt.Sprintf("/api/v1/experiments/%s/start", exp.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "RUNNING", body["data"].(map[string]interface{})["status"])
}

func TestExpStart_AlreadyRunning(t *testing.T) {
	r, svc := setupExpTestRouter()
	exp, _ := svc.Create(context.Background(), experiment.CreateInput{
		ProjectID: uuid.New(), Name: "double-start",
		ControlVersionID: uuid.New(), TreatmentVersionID: uuid.New(),
	})
	_, _ = svc.Start(context.Background(), exp.ID)
	w := doJSON(r, "POST", fmt.Sprintf("/api/v1/experiments/%s/start", exp.ID), nil)
	assert.Equal(t, http.StatusConflict, w.Code)
	errBody := parseBody(t, w)["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_TRANSITION", errBody["code"])
	assert.Contains(t, errBody["message"], "invalid status transition", "the message text is unchanged")
}

func TestExpResults_CollectingBeforeHorizon(t *testing.T) {
	r, svc := setupExpTestRouter()
	exp, _ := svc.Create(context.Background(), experiment.CreateInput{
		ProjectID: uuid.New(), Name: "results-test",
		ControlVersionID: uuid.New(), TreatmentVersionID: uuid.New(),
	})
	_, _ = svc.Start(context.Background(), exp.ID)
	w := doJSON(r, "GET", fmt.Sprintf("/api/v1/experiments/%s/results", exp.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "COLLECTING_DATA", data["recommendation"])
	assert.Nil(t, data["p_value"])
	assert.Equal(t, false, data["final"])
}

func TestExpCancel(t *testing.T) {
	r, svc := setupExpTestRouter()
	exp, _ := svc.Create(context.Background(), experiment.CreateInput{
		ProjectID: uuid.New(), Name: "cancel-test",
		ControlVersionID: uuid.New(), TreatmentVersionID: uuid.New(),
	})
	w := doJSON(r, "POST", fmt.Sprintf("/api/v1/experiments/%s/cancel", exp.ID), nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.Equal(t, "CANCELLED", body["data"].(map[string]interface{})["status"])
}

// A project_id that is not a UUID is refused, as for devices and
// deployments; a split_ratio sent as 0 is refused, one left out is 50.
func TestExperiments_ProjectFilterAndSplit(t *testing.T) {
	r, _ := setupExpTestRouter()
	w := doJSON(r, "GET", "/api/v1/experiments?project_id=abc", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "project_id must be a UUID")

	body := func(split interface{}) map[string]interface{} {
		b := map[string]interface{}{"project_id": uuid.New().String(), "name": "e-" + uuid.NewString()[:8],
			"control_version_id": uuid.New().String(), "treatment_version_id": uuid.New().String()}
		if split != nil {
			b["split_ratio"] = split
		}
		return b
	}
	w = doJSON(r, "POST", "/api/v1/experiments", body(0))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "split_ratio must be 1-99")
	w = doJSON(r, "POST", "/api/v1/experiments", body(nil))
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"split_ratio":50`)
}

// split_ratio 0 names the value given; a winner that is not a UUID says so.
func TestExperiments_SplitZeroAndWinnerMessages(t *testing.T) {
	r, _ := setupExpTestRouter()
	w := doJSON(r, "POST", "/api/v1/experiments", map[string]interface{}{"project_id": uuid.New().String(), "name": "e0",
		"control_version_id": uuid.New().String(), "treatment_version_id": uuid.New().String(), "split_ratio": 0})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "got 0")
	w = doJSON(r, "POST", "/api/v1/experiments/"+uuid.New().String()+"/conclude", map[string]interface{}{"winner_version_id": "abc"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid winner_version_id")
	w = doJSON(r, "POST", "/api/v1/experiments/"+uuid.New().String()+"/conclude", map[string]interface{}{})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "winner_version_id is required")
}

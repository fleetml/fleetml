package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/registry"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// --- Mock implementations for handler tests ---

type mockProjectRepo struct {
	projects map[uuid.UUID]*registry.Project
	slugs    map[string]uuid.UUID
}

func newMockProjectRepo() *mockProjectRepo {
	return &mockProjectRepo{projects: make(map[uuid.UUID]*registry.Project), slugs: make(map[string]uuid.UUID)}
}

func (m *mockProjectRepo) Create(_ context.Context, p *registry.Project) error {
	if _, exists := m.slugs[p.Slug]; exists {
		return fmt.Errorf("project.Create: %w: slug %q", registry.ErrConflict, p.Slug)
	}
	p.ID = uuid.New()
	m.projects[p.ID] = p
	m.slugs[p.Slug] = p.ID
	return nil
}

func (m *mockProjectRepo) GetByID(_ context.Context, id uuid.UUID) (*registry.Project, error) {
	p, ok := m.projects[id]
	if !ok {
		return nil, fmt.Errorf("project.GetByID: %w", registry.ErrNotFound)
	}
	return p, nil
}

func (m *mockProjectRepo) GetBySlug(_ context.Context, slug string) (*registry.Project, error) {
	id, ok := m.slugs[slug]
	if !ok {
		return nil, fmt.Errorf("project.GetBySlug: %w", registry.ErrNotFound)
	}
	return m.projects[id], nil
}

func (m *mockProjectRepo) List(_ context.Context, limit, offset int) ([]registry.Project, int, error) {
	var all []registry.Project
	for _, p := range m.projects {
		all = append(all, *p)
	}
	total := len(all)
	if offset >= len(all) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], total, nil
}

func (m *mockProjectRepo) Update(_ context.Context, p *registry.Project) error {
	if _, ok := m.projects[p.ID]; !ok {
		return fmt.Errorf("project.Update: %w", registry.ErrNotFound)
	}
	for slug, id := range m.slugs {
		if id == p.ID {
			delete(m.slugs, slug)
			break
		}
	}
	m.projects[p.ID] = p
	m.slugs[p.Slug] = p.ID
	return nil
}

func (m *mockProjectRepo) Delete(_ context.Context, id uuid.UUID) error {
	p, ok := m.projects[id]
	if !ok {
		return fmt.Errorf("project.Delete: %w", registry.ErrNotFound)
	}
	delete(m.slugs, p.Slug)
	delete(m.projects, id)
	return nil
}

type mockModelRepo struct {
	models map[uuid.UUID]*registry.RegistryModel
}

func newMockModelRepo() *mockModelRepo {
	return &mockModelRepo{models: make(map[uuid.UUID]*registry.RegistryModel)}
}

func (m *mockModelRepo) Create(_ context.Context, model *registry.RegistryModel) error {
	for _, existing := range m.models {
		if existing.ProjectID == model.ProjectID && existing.Name == model.Name {
			return fmt.Errorf("model.Create: %w: model %q", registry.ErrConflict, model.Name)
		}
	}
	model.ID = uuid.New()
	m.models[model.ID] = model
	return nil
}

func (m *mockModelRepo) GetByID(_ context.Context, id uuid.UUID) (*registry.RegistryModel, error) {
	model, ok := m.models[id]
	if !ok {
		return nil, fmt.Errorf("model.GetByID: %w", registry.ErrNotFound)
	}
	return model, nil
}

func (m *mockModelRepo) ListByProject(_ context.Context, projectID uuid.UUID, limit, offset int) ([]registry.RegistryModel, int, error) {
	var result []registry.RegistryModel
	for _, model := range m.models {
		if model.ProjectID == projectID {
			result = append(result, *model)
		}
	}
	total := len(result)
	if offset >= len(result) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], total, nil
}

func (m *mockModelRepo) Update(_ context.Context, model *registry.RegistryModel) error {
	if _, ok := m.models[model.ID]; !ok {
		return fmt.Errorf("model.Update: %w", registry.ErrNotFound)
	}
	m.models[model.ID] = model
	return nil
}

func (m *mockModelRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.models[id]; !ok {
		return fmt.Errorf("model.Delete: %w", registry.ErrNotFound)
	}
	delete(m.models, id)
	return nil
}

type mockVersionRepo struct {
	versions map[uuid.UUID]*registry.RegistryVersion
}

func newMockVersionRepo() *mockVersionRepo {
	return &mockVersionRepo{versions: make(map[uuid.UUID]*registry.RegistryVersion)}
}

func (m *mockVersionRepo) Create(_ context.Context, v *registry.RegistryVersion) error {
	for _, existing := range m.versions {
		if existing.ModelID == v.ModelID && existing.Version == v.Version {
			return fmt.Errorf("version.Create: %w: version %q", registry.ErrConflict, v.Version)
		}
	}
	v.ID = uuid.New()
	m.versions[v.ID] = v
	return nil
}

func (m *mockVersionRepo) GetByID(_ context.Context, id uuid.UUID) (*registry.RegistryVersion, error) {
	v, ok := m.versions[id]
	if !ok {
		return nil, fmt.Errorf("version.GetByID: %w", registry.ErrNotFound)
	}
	return v, nil
}

func (m *mockVersionRepo) GetByModelAndVersion(_ context.Context, modelID uuid.UUID, version string) (*registry.RegistryVersion, error) {
	for _, v := range m.versions {
		if v.ModelID == modelID && v.Version == version {
			return v, nil
		}
	}
	return nil, fmt.Errorf("version.GetByModelAndVersion: %w", registry.ErrNotFound)
}

func (m *mockVersionRepo) ListByModel(_ context.Context, modelID uuid.UUID, limit, offset int) ([]registry.RegistryVersion, int, error) {
	var result []registry.RegistryVersion
	for _, v := range m.versions {
		if v.ModelID == modelID {
			result = append(result, *v)
		}
	}
	total := len(result)
	if offset >= len(result) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(result) {
		end = len(result)
	}
	return result[offset:end], total, nil
}

func (m *mockVersionRepo) UpdateStatus(_ context.Context, id uuid.UUID, status string, promotedBy *string) error {
	v, ok := m.versions[id]
	if !ok {
		return fmt.Errorf("version.UpdateStatus: %w", registry.ErrNotFound)
	}
	v.Status = status
	if promotedBy != nil {
		v.PromotedBy = promotedBy
	}
	return nil
}

func (m *mockVersionRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.versions[id]; !ok {
		return fmt.Errorf("version.Delete: %w", registry.ErrNotFound)
	}
	delete(m.versions, id)
	return nil
}

type mockVariantRepo struct {
	variants map[uuid.UUID]*registry.RegistryVariant
}

func newMockVariantRepo() *mockVariantRepo {
	return &mockVariantRepo{variants: make(map[uuid.UUID]*registry.RegistryVariant)}
}

func (m *mockVariantRepo) Create(_ context.Context, v *registry.RegistryVariant) error {
	for _, existing := range m.variants {
		if existing.VersionID == v.VersionID && existing.Quantization == v.Quantization && existing.TargetRuntime == v.TargetRuntime {
			return fmt.Errorf("variant.Create: %w: variant %s/%s", registry.ErrConflict, v.Quantization, v.TargetRuntime)
		}
	}
	v.ID = uuid.New()
	m.variants[v.ID] = v
	return nil
}

func (m *mockVariantRepo) GetByID(_ context.Context, id uuid.UUID) (*registry.RegistryVariant, error) {
	v, ok := m.variants[id]
	if !ok {
		return nil, fmt.Errorf("variant.GetByID: %w", registry.ErrNotFound)
	}
	return v, nil
}

func (m *mockVariantRepo) ListByVersion(_ context.Context, versionID uuid.UUID) ([]registry.RegistryVariant, error) {
	var result []registry.RegistryVariant
	for _, v := range m.variants {
		if v.VersionID == versionID {
			result = append(result, *v)
		}
	}
	return result, nil
}

func (m *mockVariantRepo) UpdateStatus(_ context.Context, id uuid.UUID, status string) error {
	v, ok := m.variants[id]
	if !ok {
		return fmt.Errorf("variant.UpdateStatus: %w", registry.ErrNotFound)
	}
	v.Status = status
	return nil
}

func (m *mockVariantRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.variants[id]; !ok {
		return fmt.Errorf("variant.Delete: %w", registry.ErrNotFound)
	}
	delete(m.variants, id)
	return nil
}

type mockStorage struct {
	objects map[string]int64
	content map[string][]byte
}

func newMockStorage() *mockStorage {
	return &mockStorage{objects: make(map[string]int64), content: make(map[string][]byte)}
}

func (m *mockStorage) GeneratePresignedUpload(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://s3.example.com/upload/" + key, nil
}

func (m *mockStorage) GeneratePresignedDownload(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://s3.example.com/download/" + key, nil
}

func (m *mockStorage) ObjectExists(_ context.Context, key string) (bool, error) {
	_, ok := m.objects[key]
	return ok, nil
}

func (m *mockStorage) DeleteObject(_ context.Context, key string) error {
	delete(m.objects, key)
	return nil
}

// OpenObject returns the stored content, or size zero bytes if none was set.
func (m *mockStorage) OpenObject(_ context.Context, key string) (io.ReadCloser, error) {
	size, ok := m.objects[key]
	if !ok {
		return nil, fmt.Errorf("object not found")
	}
	if data, ok := m.content[key]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return io.NopCloser(bytes.NewReader(make([]byte, size))), nil
}

func (m *mockStorage) GetObjectInfo(_ context.Context, key string) (int64, error) {
	size, ok := m.objects[key]
	if !ok {
		return 0, fmt.Errorf("not found")
	}
	return size, nil
}

type mockAudit struct{}

func (m *mockAudit) Log(_ context.Context, _, _, _, _ string, _ map[string]any) error { return nil }

// --- Test helpers ---

func newTestService() *registry.Service {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return registry.NewService(
		newMockProjectRepo(),
		newMockModelRepo(),
		newMockVersionRepo(),
		newMockVariantRepo(),
		newMockStorage(),
		&mockAudit{},
		logger,
	)
}

func setupTestRouter(svc *registry.Service) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1")
	NewProjectHandler(svc, nil).RegisterRoutes(api)
	NewModelHandler(svc, nil).RegisterRoutes(api)
	NewVersionHandler(svc, nil).RegisterRoutes(api)
	NewVariantHandler(svc, nil).RegisterRoutes(api)
	return r
}

func doJSON(router *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func parseBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	return result
}

// --- Project handler tests ---

func TestProjectCRUD(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	// Create
	w := doJSON(r, "POST", "/api/v1/projects", map[string]string{
		"name":        "Fraud Detector",
		"description": "detects fraud",
	})
	assert.Equal(t, http.StatusCreated, w.Code)
	body := parseBody(t, w)
	data := body["data"].(map[string]interface{})
	assert.Equal(t, "Fraud Detector", data["name"])
	assert.Equal(t, "fraud-detector", data["slug"])
	projectID := data["id"].(string)

	// Get by ID
	w = doJSON(r, "GET", "/api/v1/projects/"+projectID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Get by slug
	w = doJSON(r, "GET", "/api/v1/projects/by-slug/fraud-detector", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// List
	w = doJSON(r, "GET", "/api/v1/projects?limit=10&offset=0", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body = parseBody(t, w)
	assert.NotNil(t, body["pagination"])

	// Update
	w = doJSON(r, "PUT", "/api/v1/projects/"+projectID, map[string]string{
		"name":        "Updated Detector",
		"description": "updated desc",
	})
	assert.Equal(t, http.StatusOK, w.Code)

	// Delete
	w = doJSON(r, "DELETE", "/api/v1/projects/"+projectID, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Get after delete should 404
	w = doJSON(r, "GET", "/api/v1/projects/"+projectID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestProjectValidation(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	// Missing name
	w := doJSON(r, "POST", "/api/v1/projects", map[string]string{})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Invalid UUID
	w = doJSON(r, "GET", "/api/v1/projects/not-a-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDuplicateProjectSlug(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	w := doJSON(r, "POST", "/api/v1/projects", map[string]string{"name": "Dup Test"})
	assert.Equal(t, http.StatusCreated, w.Code)

	w = doJSON(r, "POST", "/api/v1/projects", map[string]string{"name": "Dup Test"})
	assert.Equal(t, http.StatusConflict, w.Code)
}

// --- Model handler tests ---

func TestModelCRUD(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	// Create project first
	w := doJSON(r, "POST", "/api/v1/projects", map[string]string{"name": "ML Project"})
	require.Equal(t, http.StatusCreated, w.Code)
	projectID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	// Create model
	w = doJSON(r, "POST", "/api/v1/projects/"+projectID+"/models", map[string]string{
		"name":      "fraud-v1",
		"framework": "ONNX",
		"task_type": "CLASSIFICATION",
	})
	assert.Equal(t, http.StatusCreated, w.Code)
	modelID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	// Get model
	w = doJSON(r, "GET", "/api/v1/models/"+modelID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// List models
	w = doJSON(r, "GET", "/api/v1/projects/"+projectID+"/models", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Update model
	w = doJSON(r, "PUT", "/api/v1/models/"+modelID, map[string]string{
		"name":      "fraud-v2",
		"framework": "TFLITE",
		"task_type": "DETECTION",
	})
	assert.Equal(t, http.StatusOK, w.Code)

	// Delete model
	w = doJSON(r, "DELETE", "/api/v1/models/"+modelID, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestModelInvalidFramework(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	w := doJSON(r, "POST", "/api/v1/projects", map[string]string{"name": "P"})
	require.Equal(t, http.StatusCreated, w.Code)
	projectID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	w = doJSON(r, "POST", "/api/v1/projects/"+projectID+"/models", map[string]string{
		"name":      "test",
		"framework": "INVALID",
		"task_type": "CLASSIFICATION",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// --- Version handler tests ---

func TestVersionCRUD(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	// Setup: project + model
	w := doJSON(r, "POST", "/api/v1/projects", map[string]string{"name": "P"})
	require.Equal(t, http.StatusCreated, w.Code)
	projectID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	w = doJSON(r, "POST", "/api/v1/projects/"+projectID+"/models", map[string]string{
		"name": "m", "framework": "ONNX", "task_type": "LLM",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	modelID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	// Create version
	w = doJSON(r, "POST", "/api/v1/models/"+modelID+"/versions", map[string]string{
		"version": "1.0.0",
	})
	assert.Equal(t, http.StatusCreated, w.Code)
	verData := parseBody(t, w)["data"].(map[string]interface{})
	versionID := verData["id"].(string)
	assert.Equal(t, "DRAFT", verData["status"])

	// Get version
	w = doJSON(r, "GET", "/api/v1/versions/"+versionID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// List versions
	w = doJSON(r, "GET", "/api/v1/models/"+modelID+"/versions", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Promote to STAGED
	w = doJSON(r, "PATCH", "/api/v1/versions/"+versionID+"/status", map[string]string{
		"status": "STAGED",
	})
	assert.Equal(t, http.StatusOK, w.Code)

	// Promote to ACTIVE
	w = doJSON(r, "PATCH", "/api/v1/versions/"+versionID+"/status", map[string]string{
		"status": "ACTIVE",
	})
	assert.Equal(t, http.StatusOK, w.Code)

	// Delete
	w = doJSON(r, "DELETE", "/api/v1/versions/"+versionID, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestVersionInvalidSemver(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	w := doJSON(r, "POST", "/api/v1/projects", map[string]string{"name": "P"})
	require.Equal(t, http.StatusCreated, w.Code)
	projectID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	w = doJSON(r, "POST", "/api/v1/projects/"+projectID+"/models", map[string]string{
		"name": "m", "framework": "ONNX", "task_type": "LLM",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	modelID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	w = doJSON(r, "POST", "/api/v1/models/"+modelID+"/versions", map[string]string{
		"version": "not-semver",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVersionInvalidTransition(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	w := doJSON(r, "POST", "/api/v1/projects", map[string]string{"name": "P"})
	require.Equal(t, http.StatusCreated, w.Code)
	projectID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	w = doJSON(r, "POST", "/api/v1/projects/"+projectID+"/models", map[string]string{
		"name": "m", "framework": "ONNX", "task_type": "LLM",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	modelID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	w = doJSON(r, "POST", "/api/v1/models/"+modelID+"/versions", map[string]string{
		"version": "1.0.0",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	versionID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	// DRAFT -> ACTIVE should fail
	w = doJSON(r, "PATCH", "/api/v1/versions/"+versionID+"/status", map[string]string{
		"status": "ACTIVE",
	})
	assert.Equal(t, http.StatusConflict, w.Code)
	errBody := parseBody(t, w)["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_TRANSITION", errBody["code"])
	assert.Equal(t, "invalid status transition: cannot transition from DRAFT to ACTIVE", errBody["message"])
}

// A limit or offset that is not an integer is refused, naming the parameter;
// before, it was ignored and the default page came back.
func TestPagination_NotAnInteger(t *testing.T) {
	r := setupTestRouter(newTestService())
	for q, want := range map[string]string{
		"limit=abc":        `limit must be an integer, got "abc"`,
		"offset=1.5":       `offset must be an integer, got "1.5"`,
		"limit=5&offset=x": `offset must be an integer, got "x"`,
	} {
		w := doJSON(r, "GET", "/api/v1/projects?"+q, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
		errBody := parseBody(t, w)["error"].(map[string]interface{})
		assert.Equal(t, "VALIDATION_ERROR", errBody["code"], q)
		assert.Equal(t, want, errBody["message"], q)
	}
	w := doJSON(r, "GET", "/api/v1/projects?limit=5&offset=0", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

// A page size outside 1 to the cap, or a negative offset, is refused with the
// range; before 0.9.1 it was clamped without a word.
func TestPagination_OutOfRange(t *testing.T) {
	r := setupTestRouter(newTestService())
	for q, want := range map[string]string{
		"limit=101":  "limit must be between 1 and 100, got 101",
		"limit=0":    "limit must be between 1 and 100, got 0",
		"limit=-1":   "limit must be between 1 and 100, got -1",
		"offset=-3":  "offset must be 0 or more, got -3",
		"limit=1000": "limit must be between 1 and 100, got 1000",
	} {
		w := doJSON(r, "GET", "/api/v1/projects?"+q, nil)
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
		errBody := parseBody(t, w)["error"].(map[string]interface{})
		assert.Equal(t, "VALIDATION_ERROR", errBody["code"], q)
		assert.Equal(t, want, errBody["message"], q)
	}
	for _, q := range []string{"limit=100", "limit=1", "offset=0"} {
		assert.Equal(t, http.StatusOK, doJSON(r, "GET", "/api/v1/projects?"+q, nil).Code, q)
	}
}

// An unknown status is invalid input, not a transition the version's status
// does not allow.
func TestVersionStatus_Unknown(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)
	w := doJSON(r, "POST", "/api/v1/projects", map[string]string{"name": "P"})
	projectID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)
	w = doJSON(r, "POST", "/api/v1/projects/"+projectID+"/models", map[string]string{"name": "m", "framework": "TFLITE", "task_type": "CLASSIFICATION"})
	modelID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)
	w = doJSON(r, "POST", "/api/v1/models/"+modelID+"/versions", map[string]string{"version": "1.0.0"})
	versionID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	w = doJSON(r, "PATCH", "/api/v1/versions/"+versionID+"/status", map[string]string{"status": "LIVE"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	errBody := parseBody(t, w)["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errBody["code"])
	assert.Equal(t, `invalid input: status must be one of DRAFT, STAGED, ACTIVE, ARCHIVED, got "LIVE"`, errBody["message"])
}

// --- Variant handler tests ---

func TestVariantCRUD(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	// Setup: project + model + version
	w := doJSON(r, "POST", "/api/v1/projects", map[string]string{"name": "P"})
	require.Equal(t, http.StatusCreated, w.Code)
	projectID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	w = doJSON(r, "POST", "/api/v1/projects/"+projectID+"/models", map[string]string{
		"name": "m", "framework": "ONNX", "task_type": "LLM",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	modelID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	w = doJSON(r, "POST", "/api/v1/models/"+modelID+"/versions", map[string]string{
		"version": "1.0.0",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	versionID := parseBody(t, w)["data"].(map[string]interface{})["id"].(string)

	// Create variant
	w = doJSON(r, "POST", "/api/v1/versions/"+versionID+"/variants", map[string]interface{}{
		"quantization":    "INT8",
		"target_runtime":  "TFLITE",
		"blob_size_bytes": 1024,
		"blob_sha256":     "abc123",
	})
	assert.Equal(t, http.StatusCreated, w.Code)
	body := parseBody(t, w)
	variantData := body["data"].(map[string]interface{})
	assert.NotEmpty(t, variantData["upload_url"])
	variant := variantData["variant"].(map[string]interface{})
	variantID := variant["id"].(string)

	// List variants
	w = doJSON(r, "GET", "/api/v1/versions/"+versionID+"/variants", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Get variant
	w = doJSON(r, "GET", "/api/v1/variants/"+variantID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Download URL
	w = doJSON(r, "GET", "/api/v1/variants/"+variantID+"/download", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	dlBody := parseBody(t, w)
	dlData := dlBody["data"].(map[string]interface{})
	assert.NotEmpty(t, dlData["download_url"])

	// Delete variant
	w = doJSON(r, "DELETE", "/api/v1/variants/"+variantID, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestVariantNotFound(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	w := doJSON(r, "GET", fmt.Sprintf("/api/v1/variants/%s", "00000000-0000-0000-0000-000000000001"), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPaginationDefaults(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	w := doJSON(r, "GET", "/api/v1/projects", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	pagination := body["pagination"].(map[string]interface{})
	assert.Equal(t, float64(20), pagination["limit"])
	assert.Equal(t, float64(0), pagination["offset"])
}

func TestErrorResponseFormat(t *testing.T) {
	svc := newTestService()
	r := setupTestRouter(svc)

	w := doJSON(r, "GET", "/api/v1/projects/not-a-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := parseBody(t, w)
	errObj := body["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	assert.NotEmpty(t, errObj["message"])
}

package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock implementations ---

type mockProjectRepo struct {
	projects map[uuid.UUID]*Project
	slugs    map[string]uuid.UUID
}

func newMockProjectRepo() *mockProjectRepo {
	return &mockProjectRepo{
		projects: make(map[uuid.UUID]*Project),
		slugs:    make(map[string]uuid.UUID),
	}
}

func (m *mockProjectRepo) Create(_ context.Context, p *Project) error {
	if _, exists := m.slugs[p.Slug]; exists {
		return fmt.Errorf("project.Create: %w: slug %q", ErrConflict, p.Slug)
	}
	p.ID = uuid.New()
	m.projects[p.ID] = p
	m.slugs[p.Slug] = p.ID
	return nil
}

func (m *mockProjectRepo) GetByID(_ context.Context, id uuid.UUID) (*Project, error) {
	p, ok := m.projects[id]
	if !ok {
		return nil, fmt.Errorf("project.GetByID: %w", ErrNotFound)
	}
	return p, nil
}

func (m *mockProjectRepo) GetBySlug(_ context.Context, slug string) (*Project, error) {
	id, ok := m.slugs[slug]
	if !ok {
		return nil, fmt.Errorf("project.GetBySlug: %w", ErrNotFound)
	}
	return m.projects[id], nil
}

func (m *mockProjectRepo) List(_ context.Context, limit, offset int) ([]Project, int, error) {
	var all []Project
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

func (m *mockProjectRepo) Update(_ context.Context, p *Project) error {
	if _, ok := m.projects[p.ID]; !ok {
		return fmt.Errorf("project.Update: %w", ErrNotFound)
	}
	// Check slug uniqueness (ignore self)
	if existingID, exists := m.slugs[p.Slug]; exists && existingID != p.ID {
		return fmt.Errorf("project.Update: %w: slug %q", ErrConflict, p.Slug)
	}
	// Remove old slug
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
		return fmt.Errorf("project.Delete: %w", ErrNotFound)
	}
	delete(m.slugs, p.Slug)
	delete(m.projects, id)
	return nil
}

type mockModelRepo struct {
	models map[uuid.UUID]*RegistryModel
}

func newMockModelRepo() *mockModelRepo {
	return &mockModelRepo{models: make(map[uuid.UUID]*RegistryModel)}
}

func (m *mockModelRepo) Create(_ context.Context, model *RegistryModel) error {
	for _, existing := range m.models {
		if existing.ProjectID == model.ProjectID && existing.Name == model.Name {
			return fmt.Errorf("model.Create: %w: model %q in project", ErrConflict, model.Name)
		}
	}
	model.ID = uuid.New()
	m.models[model.ID] = model
	return nil
}

func (m *mockModelRepo) GetByID(_ context.Context, id uuid.UUID) (*RegistryModel, error) {
	model, ok := m.models[id]
	if !ok {
		return nil, fmt.Errorf("model.GetByID: %w", ErrNotFound)
	}
	return model, nil
}

func (m *mockModelRepo) ListByProject(_ context.Context, projectID uuid.UUID, limit, offset int) ([]RegistryModel, int, error) {
	var result []RegistryModel
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

func (m *mockModelRepo) Update(_ context.Context, model *RegistryModel) error {
	if _, ok := m.models[model.ID]; !ok {
		return fmt.Errorf("model.Update: %w", ErrNotFound)
	}
	m.models[model.ID] = model
	return nil
}

func (m *mockModelRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.models[id]; !ok {
		return fmt.Errorf("model.Delete: %w", ErrNotFound)
	}
	delete(m.models, id)
	return nil
}

type mockVersionRepo struct {
	versions map[uuid.UUID]*RegistryVersion
}

func newMockVersionRepo() *mockVersionRepo {
	return &mockVersionRepo{versions: make(map[uuid.UUID]*RegistryVersion)}
}

func (m *mockVersionRepo) Create(_ context.Context, v *RegistryVersion) error {
	for _, existing := range m.versions {
		if existing.ModelID == v.ModelID && existing.Version == v.Version {
			return fmt.Errorf("version.Create: %w: version %q for model", ErrConflict, v.Version)
		}
	}
	v.ID = uuid.New()
	m.versions[v.ID] = v
	return nil
}

func (m *mockVersionRepo) GetByID(_ context.Context, id uuid.UUID) (*RegistryVersion, error) {
	v, ok := m.versions[id]
	if !ok {
		return nil, fmt.Errorf("version.GetByID: %w", ErrNotFound)
	}
	return v, nil
}

func (m *mockVersionRepo) GetByModelAndVersion(_ context.Context, modelID uuid.UUID, version string) (*RegistryVersion, error) {
	for _, v := range m.versions {
		if v.ModelID == modelID && v.Version == version {
			return v, nil
		}
	}
	return nil, fmt.Errorf("version.GetByModelAndVersion: %w", ErrNotFound)
}

func (m *mockVersionRepo) ListByModel(_ context.Context, modelID uuid.UUID, limit, offset int) ([]RegistryVersion, int, error) {
	var result []RegistryVersion
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
		return fmt.Errorf("version.UpdateStatus: %w", ErrNotFound)
	}
	v.Status = status
	if promotedBy != nil {
		v.PromotedBy = promotedBy
	}
	return nil
}

func (m *mockVersionRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.versions[id]; !ok {
		return fmt.Errorf("version.Delete: %w", ErrNotFound)
	}
	delete(m.versions, id)
	return nil
}

type mockVariantRepo struct {
	variants map[uuid.UUID]*RegistryVariant
}

func newMockVariantRepo() *mockVariantRepo {
	return &mockVariantRepo{variants: make(map[uuid.UUID]*RegistryVariant)}
}

func (m *mockVariantRepo) Create(_ context.Context, v *RegistryVariant) error {
	for _, existing := range m.variants {
		if existing.VersionID == v.VersionID && existing.Quantization == v.Quantization && existing.TargetRuntime == v.TargetRuntime {
			return fmt.Errorf("variant.Create: %w: variant %s/%s", ErrConflict, v.Quantization, v.TargetRuntime)
		}
	}
	v.ID = uuid.New()
	m.variants[v.ID] = v
	return nil
}

func (m *mockVariantRepo) GetByID(_ context.Context, id uuid.UUID) (*RegistryVariant, error) {
	v, ok := m.variants[id]
	if !ok {
		return nil, fmt.Errorf("variant.GetByID: %w", ErrNotFound)
	}
	return v, nil
}

func (m *mockVariantRepo) ListByVersion(_ context.Context, versionID uuid.UUID) ([]RegistryVariant, error) {
	var result []RegistryVariant
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
		return fmt.Errorf("variant.UpdateStatus: %w", ErrNotFound)
	}
	v.Status = status
	return nil
}

func (m *mockVariantRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.variants[id]; !ok {
		return fmt.Errorf("variant.Delete: %w", ErrNotFound)
	}
	delete(m.variants, id)
	return nil
}

type mockStorage struct {
	objects map[string]int64  // key -> size
	content map[string][]byte // key -> bytes (optional; zeros of size otherwise)
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
		return 0, fmt.Errorf("object not found")
	}
	return size, nil
}

type mockAudit struct{}

func (m *mockAudit) Log(_ context.Context, _, _, _, _ string, _ map[string]any) error {
	return nil
}

// --- Test helpers ---

type testEnv struct {
	svc      *Service
	projects *mockProjectRepo
	models   *mockModelRepo
	versions *mockVersionRepo
	variants *mockVariantRepo
	storage  *mockStorage
}

func newTestEnv() *testEnv {
	pr := newMockProjectRepo()
	mr := newMockModelRepo()
	vr := newMockVersionRepo()
	varr := newMockVariantRepo()
	st := newMockStorage()
	svc := NewService(pr, mr, vr, varr, st, &mockAudit{}, nil)
	return &testEnv{svc: svc, projects: pr, models: mr, versions: vr, variants: varr, storage: st}
}

func createTestProject(t *testing.T, svc *Service) *Project {
	t.Helper()
	p, err := svc.CreateProject(context.Background(), "Test Project", "desc")
	require.NoError(t, err)
	return p
}

func createTestModel(t *testing.T, svc *Service, projectID uuid.UUID) *RegistryModel {
	t.Helper()
	m, err := svc.CreateModel(context.Background(), projectID, "test-model", "desc", "ONNX", "CLASSIFICATION", "tester")
	require.NoError(t, err)
	return m
}

func createTestVersion(t *testing.T, svc *Service, modelID uuid.UUID) *RegistryVersion {
	t.Helper()
	v, err := svc.CreateVersion(context.Background(), modelID, "1.0.0", "notes", nil)
	require.NoError(t, err)
	return v
}

// --- Tests ---

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "Fraud Detector", "fraud-detector"},
		{"with spaces", "  My Project  ", "my-project"},
		{"special chars", "Hello World! #1", "hello-world-1"},
		{"numeric start", "123 project", "p-123-project"},
		{"long name", string(make([]byte, 200)), "project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug := GenerateSlug(tt.input)
			assert.NotEmpty(t, slug)
			if tt.expected != "" {
				assert.Equal(t, tt.expected, slug)
			}
			assert.LessOrEqual(t, len(slug), 63)
		})
	}
}

func TestCreateProject(t *testing.T) {
	env := newTestEnv()
	svc := env.svc
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		p, err := svc.CreateProject(ctx, "Fraud Detector", "detects fraud")
		require.NoError(t, err)
		assert.Equal(t, "Fraud Detector", p.Name)
		assert.Equal(t, "fraud-detector", p.Slug)
		assert.NotEqual(t, uuid.Nil, p.ID)
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := svc.CreateProject(ctx, "", "desc")
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("name too long", func(t *testing.T) {
		_, err := svc.CreateProject(ctx, string(make([]byte, 256)), "desc")
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("duplicate slug", func(t *testing.T) {
		_, err := svc.CreateProject(ctx, "Fraud Detector", "another one")
		assert.ErrorIs(t, err, ErrConflict)
	})
}

func TestCreateModel(t *testing.T) {
	env := newTestEnv()
	svc := env.svc
	ctx := context.Background()
	proj := createTestProject(t, svc)

	t.Run("success", func(t *testing.T) {
		m, err := svc.CreateModel(ctx, proj.ID, "fraud-v1", "model desc", "ONNX", "CLASSIFICATION", "tester")
		require.NoError(t, err)
		assert.Equal(t, "fraud-v1", m.Name)
		assert.Equal(t, "ONNX", m.Framework)
	})

	t.Run("invalid framework", func(t *testing.T) {
		_, err := svc.CreateModel(ctx, proj.ID, "test", "desc", "INVALID", "CLASSIFICATION", "tester")
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("invalid task type", func(t *testing.T) {
		_, err := svc.CreateModel(ctx, proj.ID, "test", "desc", "ONNX", "INVALID", "tester")
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("nonexistent project", func(t *testing.T) {
		_, err := svc.CreateModel(ctx, uuid.New(), "test", "desc", "ONNX", "CLASSIFICATION", "tester")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := svc.CreateModel(ctx, proj.ID, "", "desc", "ONNX", "CLASSIFICATION", "tester")
		assert.ErrorIs(t, err, ErrInvalidInput)
	})
}

func TestCreateVersion(t *testing.T) {
	env := newTestEnv()
	svc := env.svc
	ctx := context.Background()
	proj := createTestProject(t, svc)
	model := createTestModel(t, svc, proj.ID)

	t.Run("success", func(t *testing.T) {
		v, err := svc.CreateVersion(ctx, model.ID, "1.0.0", "first release", nil)
		require.NoError(t, err)
		assert.Equal(t, "DRAFT", v.Status)
		assert.Equal(t, "1.0.0", v.Version)
	})

	t.Run("invalid semver", func(t *testing.T) {
		_, err := svc.CreateVersion(ctx, model.ID, "not-semver", "", nil)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("nonexistent model", func(t *testing.T) {
		_, err := svc.CreateVersion(ctx, uuid.New(), "1.0.0", "", nil)
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("duplicate version", func(t *testing.T) {
		_, err := svc.CreateVersion(ctx, model.ID, "1.0.0", "", nil)
		assert.ErrorIs(t, err, ErrConflict)
	})
}

func TestStatusTransitions(t *testing.T) {
	validCases := []struct {
		name string
		from string
		to   string
	}{
		{"draft to staged", "DRAFT", "STAGED"},
		{"staged to active", "STAGED", "ACTIVE"},
		{"active to archived", "ACTIVE", "ARCHIVED"},
		{"draft to archived", "DRAFT", "ARCHIVED"},
	}

	invalidCases := []struct {
		name string
		from string
		to   string
	}{
		{"draft to active", "DRAFT", "ACTIVE"},
		{"staged to draft", "STAGED", "DRAFT"},
		{"active to staged", "ACTIVE", "STAGED"},
		{"archived to anything", "ARCHIVED", "DRAFT"},
		{"staged to archived", "STAGED", "ARCHIVED"},
	}

	for _, tt := range validCases {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv()
			svc := env.svc
			vr := env.versions
			ctx := context.Background()
			proj := createTestProject(t, svc)
			model := createTestModel(t, svc, proj.ID)
			ver := createTestVersion(t, svc, model.ID)

			// Set the version to the starting status
			vr.versions[ver.ID].Status = tt.from

			_, err := svc.UpdateVersionStatus(ctx, ver.ID, tt.to, nil)
			assert.NoError(t, err)
		})
	}

	for _, tt := range invalidCases {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv()
			svc := env.svc
			vr := env.versions
			ctx := context.Background()
			proj := createTestProject(t, svc)
			model := createTestModel(t, svc, proj.ID)
			ver := createTestVersion(t, svc, model.ID)

			vr.versions[ver.ID].Status = tt.from

			_, err := svc.UpdateVersionStatus(ctx, ver.ID, tt.to, nil)
			assert.ErrorIs(t, err, ErrInvalidTransition)
		})
	}
}

func TestCreateVariant(t *testing.T) {
	env := newTestEnv()
	svc := env.svc
	ctx := context.Background()
	proj := createTestProject(t, svc)
	model := createTestModel(t, svc, proj.ID)
	ver := createTestVersion(t, svc, model.ID)

	t.Run("success", func(t *testing.T) {
		out, err := svc.CreateVariant(ctx, CreateVariantInput{
			VersionID:     ver.ID,
			Quantization:  "INT8",
			TargetRuntime: "TFLITE",
			BlobSizeBytes: 1024,
			BlobSHA256:    "abc123",
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, out.Variant.ID)
		assert.Contains(t, out.UploadURL, "upload")
		assert.Contains(t, out.Variant.BlobKey, "INT8_TFLITE.bin")
	})

	t.Run("invalid quantization", func(t *testing.T) {
		_, err := svc.CreateVariant(ctx, CreateVariantInput{
			VersionID:     ver.ID,
			Quantization:  "INVALID",
			TargetRuntime: "TFLITE",
			BlobSizeBytes: 1024,
			BlobSHA256:    "abc",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("invalid runtime", func(t *testing.T) {
		_, err := svc.CreateVariant(ctx, CreateVariantInput{
			VersionID:     ver.ID,
			Quantization:  "FP32",
			TargetRuntime: "INVALID",
			BlobSizeBytes: 1024,
			BlobSHA256:    "abc",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("zero blob size", func(t *testing.T) {
		_, err := svc.CreateVariant(ctx, CreateVariantInput{
			VersionID:     ver.ID,
			Quantization:  "FP32",
			TargetRuntime: "ONNX",
			BlobSizeBytes: 0,
			BlobSHA256:    "abc",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("empty sha256", func(t *testing.T) {
		_, err := svc.CreateVariant(ctx, CreateVariantInput{
			VersionID:     ver.ID,
			Quantization:  "FP32",
			TargetRuntime: "ONNX",
			BlobSizeBytes: 1024,
			BlobSHA256:    "",
		})
		assert.ErrorIs(t, err, ErrInvalidInput)
	})
}

// A push whose upload failed (or whose finalize refused the blob) can be
// retried: the next create for the same quantization and runtime replaces the
// unfinalized variant. A READY variant is never replaced.
func TestCreateVariantRetryAfterFailedUpload(t *testing.T) {
	env := newTestEnv()
	svc := env.svc
	ctx := context.Background()
	ver := createTestVersion(t, svc, createTestModel(t, svc, createTestProject(t, svc).ID).ID)
	blob := bytes.Repeat([]byte("model-bytes-"), 100)
	sum := sha256.Sum256(blob)
	input := CreateVariantInput{VersionID: ver.ID, Quantization: "INT8", TargetRuntime: "TFLITE",
		BlobSizeBytes: int64(len(blob)), BlobSHA256: hex.EncodeToString(sum[:])}

	first, err := svc.CreateVariant(ctx, input)
	require.NoError(t, err)

	// The upload never reached the object store; the push is run again.
	second, err := svc.CreateVariant(ctx, input)
	require.NoError(t, err, "a retry after a failed upload must not be refused")
	assert.NotEqual(t, first.Variant.ID, second.Variant.ID)
	assert.NotEmpty(t, second.UploadURL)
	assert.Equal(t, VariantUploading, second.Variant.Status)
	assert.NotContains(t, env.variants.variants, first.Variant.ID, "the unfinalized variant is replaced, not kept")
	assert.Len(t, env.variants.variants, 1)

	// A FAILED variant (finalize refused the blob) is replaced the same way.
	env.storage.objects[second.Variant.BlobKey] = 1
	_, err = svc.FinalizeVariant(ctx, second.Variant.ID)
	require.ErrorIs(t, err, ErrIntegrity)
	third, err := svc.CreateVariant(ctx, input)
	require.NoError(t, err)

	env.storage.objects[third.Variant.BlobKey] = int64(len(blob))
	env.storage.content[third.Variant.BlobKey] = blob
	_, err = svc.FinalizeVariant(ctx, third.Variant.ID)
	require.NoError(t, err)

	_, err = svc.CreateVariant(ctx, input)
	assert.ErrorIs(t, err, ErrConflict, "a READY variant is never replaced")
	assert.Contains(t, err.Error(), "already uploaded and verified")
	assert.Equal(t, VariantReady, env.variants.variants[third.Variant.ID].Status)

	// Another quantization of the same version is a new variant, not a replacement.
	other := input
	other.Quantization = "FP16"
	_, err = svc.CreateVariant(ctx, other)
	require.NoError(t, err)
	assert.Len(t, env.variants.variants, 2)
}

func TestFinalizeVariant(t *testing.T) {
	env := newTestEnv()
	svc := env.svc
	storage := env.storage
	ctx := context.Background()
	proj := createTestProject(t, svc)
	model := createTestModel(t, svc, proj.ID)
	ver := createTestVersion(t, svc, model.ID)

	blob := bytes.Repeat([]byte("model-bytes-"), 100)
	sum := sha256.Sum256(blob)
	out, err := svc.CreateVariant(ctx, CreateVariantInput{
		VersionID:     ver.ID,
		Quantization:  "INT8",
		TargetRuntime: "TFLITE",
		BlobSizeBytes: int64(len(blob)),
		BlobSHA256:    hex.EncodeToString(sum[:]),
	})
	require.NoError(t, err)
	assert.Equal(t, VariantUploading, env.variants.variants[out.Variant.ID].Status, "new variants are not assignable")
	key := out.Variant.BlobKey

	t.Run("blob not uploaded", func(t *testing.T) {
		_, err := svc.FinalizeVariant(ctx, out.Variant.ID)
		assert.ErrorIs(t, err, ErrInvalidInput)
	})

	t.Run("size mismatch deletes the blob and fails the variant", func(t *testing.T) {
		storage.objects[key] = 999
		_, err := svc.FinalizeVariant(ctx, out.Variant.ID)
		assert.ErrorIs(t, err, ErrIntegrity)
		assert.NotContains(t, storage.objects, key)
		assert.Equal(t, VariantFailed, env.variants.variants[out.Variant.ID].Status)
	})

	t.Run("same size, wrong content", func(t *testing.T) {
		tampered := bytes.Repeat([]byte("other-bytes-"), 100)
		storage.objects[key] = int64(len(tampered))
		storage.content[key] = tampered
		_, err := svc.FinalizeVariant(ctx, out.Variant.ID)
		assert.ErrorIs(t, err, ErrIntegrity)
		assert.Contains(t, err.Error(), "sha256")
		assert.NotContains(t, storage.objects, key, "a blob that fails verification is deleted")
		assert.Equal(t, VariantFailed, env.variants.variants[out.Variant.ID].Status)
	})

	t.Run("re-upload with the right bytes succeeds", func(t *testing.T) {
		storage.objects[key] = int64(len(blob))
		storage.content[key] = blob
		v, err := svc.FinalizeVariant(ctx, out.Variant.ID)
		require.NoError(t, err)
		assert.Equal(t, out.Variant.ID, v.ID)
		assert.Equal(t, VariantReady, v.Status)
		assert.Equal(t, VariantReady, env.variants.variants[out.Variant.ID].Status)
	})

	t.Run("declared hash is compared case-insensitively", func(t *testing.T) {
		env.variants.variants[out.Variant.ID].BlobSHA256 = strings.ToUpper(hex.EncodeToString(sum[:]))
		_, err := svc.FinalizeVariant(ctx, out.Variant.ID)
		require.NoError(t, err)
	})
}

func TestDeleteVariant(t *testing.T) {
	env := newTestEnv()
	svc := env.svc
	storage := env.storage
	ctx := context.Background()
	proj := createTestProject(t, svc)
	model := createTestModel(t, svc, proj.ID)
	ver := createTestVersion(t, svc, model.ID)

	out, err := svc.CreateVariant(ctx, CreateVariantInput{
		VersionID:     ver.ID,
		Quantization:  "FP16",
		TargetRuntime: "ONNX",
		BlobSizeBytes: 2048,
		BlobSHA256:    "def456",
	})
	require.NoError(t, err)
	storage.objects[out.Variant.BlobKey] = 2048

	err = svc.DeleteVariant(ctx, out.Variant.ID)
	require.NoError(t, err)

	// S3 object should be deleted
	_, exists := storage.objects[out.Variant.BlobKey]
	assert.False(t, exists)

	// Variant should not be found
	_, err = svc.GetVariant(ctx, out.Variant.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestListProjects(t *testing.T) {
	env := newTestEnv()
	svc := env.svc
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		_, err := svc.CreateProject(ctx, fmt.Sprintf("Project %d", i), "")
		require.NoError(t, err)
	}

	projects, total, err := svc.ListProjects(ctx, 3, 0)
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, projects, 3)
}

func TestGetVariantDownloadURL(t *testing.T) {
	env := newTestEnv()
	svc := env.svc
	ctx := context.Background()
	proj := createTestProject(t, svc)
	model := createTestModel(t, svc, proj.ID)
	ver := createTestVersion(t, svc, model.ID)

	out, err := svc.CreateVariant(ctx, CreateVariantInput{
		VersionID:     ver.ID,
		Quantization:  "FP32",
		TargetRuntime: "ONNX",
		BlobSizeBytes: 1024,
		BlobSHA256:    "sha",
	})
	require.NoError(t, err)

	url, err := svc.GetVariantDownloadURL(ctx, out.Variant.ID)
	require.NoError(t, err)
	assert.Contains(t, url, "download")
}

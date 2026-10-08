package intelligence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/device"
)

type mockVariantRepo struct {
	variants map[uuid.UUID][]VariantData
}

func (m *mockVariantRepo) GetByVersionID(_ context.Context, versionID uuid.UUID) ([]VariantData, error) {
	return m.variants[versionID], nil
}

type mockStorageClient struct{}

func (m *mockStorageClient) GeneratePresignedDownload(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://s3.example.com/" + key, nil
}

func newTestOrchestrator(variants map[uuid.UUID][]VariantData) *Orchestrator {
	return NewOrchestrator(
		NewProfiler(nil),
		NewSelector(nil),
		&mockVariantRepo{variants: variants},
		&mockStorageClient{},
		nil,
	)
}

func TestComputeAssignments_FlagshipGetsFP32(t *testing.T) {
	versionID := uuid.New()
	fp32ID := uuid.New()
	int8ID := uuid.New()

	orch := newTestOrchestrator(map[uuid.UUID][]VariantData{
		versionID: {
			{ID: fp32ID, VersionID: versionID, Quantization: "FP32", Runtime: "TFLITE", BlobKey: "fp32.bin", BlobSize: 100 * 1024 * 1024, BlobSHA256: "sha-fp32"},
			{ID: int8ID, VersionID: versionID, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "int8.bin", BlobSize: 25 * 1024 * 1024, BlobSHA256: "sha-int8"},
		},
	})

	dev := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 12288, HasNPU: true, NPUTflops: 45, HasGPU: true, Labels: map[string]string{}}
	deployments := []DeploymentData{
		{ID: uuid.New(), ModelVersionID: versionID, Strategy: "ALL_AT_ONCE"},
	}

	assignments, err := orch.ComputeAssignments(context.Background(), dev, nil, deployments, nil)
	require.NoError(t, err)
	require.Len(t, assignments, 1)
	assert.Equal(t, "LOAD", assignments[0].Action)
	assert.Equal(t, fp32ID.String(), assignments[0].VariantID)
	assert.NotEmpty(t, assignments[0].DownloadURL)
}

func TestComputeAssignments_MidrangeGetsINT8(t *testing.T) {
	versionID := uuid.New()
	fp32ID := uuid.New()
	int8ID := uuid.New()

	orch := newTestOrchestrator(map[uuid.UUID][]VariantData{
		versionID: {
			{ID: fp32ID, VersionID: versionID, Quantization: "FP32", Runtime: "TFLITE", BlobKey: "fp32.bin", BlobSize: 100 * 1024 * 1024, BlobSHA256: "sha-fp32"},
			{ID: int8ID, VersionID: versionID, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "int8.bin", BlobSize: 25 * 1024 * 1024, BlobSHA256: "sha-int8"},
		},
	})

	dev := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 4096, HasGPU: true, Labels: map[string]string{}}
	deployments := []DeploymentData{
		{ID: uuid.New(), ModelVersionID: versionID, Strategy: "ALL_AT_ONCE"},
	}

	assignments, err := orch.ComputeAssignments(context.Background(), dev, nil, deployments, nil)
	require.NoError(t, err)
	require.Len(t, assignments, 1)
	assert.Equal(t, int8ID.String(), assignments[0].VariantID)
}

func TestComputeAssignments_AlreadyLoaded(t *testing.T) {
	versionID := uuid.New()
	int8ID := uuid.New()

	orch := newTestOrchestrator(map[uuid.UUID][]VariantData{
		versionID: {
			{ID: int8ID, VersionID: versionID, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "int8.bin", BlobSize: 25 * 1024 * 1024, BlobSHA256: "sha-int8"},
		},
	})

	dev := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 4096, HasGPU: true, Labels: map[string]string{}}
	deployments := []DeploymentData{
		{ID: uuid.New(), ModelVersionID: versionID, Strategy: "ALL_AT_ONCE"},
	}
	loaded := []LoadedModelInfo{{VariantID: int8ID.String()}}

	assignments, err := orch.ComputeAssignments(context.Background(), dev, nil, deployments, loaded)
	require.NoError(t, err)
	assert.Empty(t, assignments)
}

func TestComputeAssignments_UnloadRemovedDeployment(t *testing.T) {
	orch := newTestOrchestrator(nil)

	dev := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 4096, Labels: map[string]string{}}
	loaded := []LoadedModelInfo{{ModelID: "old-model", VariantID: "old-variant"}}

	assignments, err := orch.ComputeAssignments(context.Background(), dev, nil, nil, loaded)
	require.NoError(t, err)
	require.Len(t, assignments, 1)
	assert.Equal(t, "UNLOAD", assignments[0].Action)
}

func TestComputeAssignments_TargetingSelectorFilters(t *testing.T) {
	versionID := uuid.New()
	orch := newTestOrchestrator(map[uuid.UUID][]VariantData{
		versionID: {
			{ID: uuid.New(), VersionID: versionID, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "int8.bin", BlobSize: 25 * 1024 * 1024, BlobSHA256: "sha"},
		},
	})

	dev := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 4096, Labels: map[string]string{"env": "prod"}}
	deployments := []DeploymentData{
		{ID: uuid.New(), ModelVersionID: versionID, Strategy: "ALL_AT_ONCE",
			TargetSelector: TargetSelector{Labels: map[string]string{"env": "staging"}}},
	}

	assignments, err := orch.ComputeAssignments(context.Background(), dev, nil, deployments, nil)
	require.NoError(t, err)
	assert.Empty(t, assignments)
}

func TestComputeAssignments_CanaryFiltering(t *testing.T) {
	versionID := uuid.New()
	orch := newTestOrchestrator(map[uuid.UUID][]VariantData{
		versionID: {
			{ID: uuid.New(), VersionID: versionID, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "int8.bin", BlobSize: 25 * 1024 * 1024, BlobSHA256: "sha"},
		},
	})

	depID := uuid.New()
	// Try many devices - some should be in canary, some not
	inCanary := 0
	for i := 0; i < 100; i++ {
		dev := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 4096, HasGPU: true, Labels: map[string]string{}}
		deployments := []DeploymentData{
			{ID: depID, ModelVersionID: versionID, Strategy: "CANARY", CanaryPercent: 50},
		}
		assignments, err := orch.ComputeAssignments(context.Background(), dev, nil, deployments, nil)
		require.NoError(t, err)
		if len(assignments) > 0 {
			inCanary++
		}
	}
	// With 50% canary and 100 devices, expect roughly 40-60 in canary
	assert.Greater(t, inCanary, 20, "too few in canary")
	assert.Less(t, inCanary, 80, "too many in canary")
}

func TestComputeAssignments_NoCompatibleVariant(t *testing.T) {
	versionID := uuid.New()
	orch := newTestOrchestrator(map[uuid.UUID][]VariantData{
		versionID: {
			{ID: uuid.New(), VersionID: versionID, Quantization: "FP32", Runtime: "COREML", BlobKey: "fp32.bin", BlobSize: 100 * 1024 * 1024, BlobSHA256: "sha"},
		},
	})

	dev := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 12288, HasNPU: true, NPUTflops: 45, Labels: map[string]string{}}
	deployments := []DeploymentData{
		{ID: uuid.New(), ModelVersionID: versionID, Strategy: "ALL_AT_ONCE"},
	}

	assignments, err := orch.ComputeAssignments(context.Background(), dev, nil, deployments, nil)
	require.NoError(t, err)
	assert.Empty(t, assignments)
}

func TestComputeAssignments_MultipleDeployments(t *testing.T) {
	v1 := uuid.New()
	v2 := uuid.New()

	orch := newTestOrchestrator(map[uuid.UUID][]VariantData{
		v1: {{ID: uuid.New(), VersionID: v1, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "m1.bin", BlobSize: 1024, BlobSHA256: "sha1"}},
		v2: {{ID: uuid.New(), VersionID: v2, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "m2.bin", BlobSize: 2048, BlobSHA256: "sha2"}},
	})

	dev := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 4096, HasGPU: true, Labels: map[string]string{}}
	deployments := []DeploymentData{
		{ID: uuid.New(), ModelVersionID: v1, ModelName: "model-a", Strategy: "ALL_AT_ONCE"},
		{ID: uuid.New(), ModelVersionID: v2, ModelName: "model-b", Strategy: "ALL_AT_ONCE"},
	}

	assignments, err := orch.ComputeAssignments(context.Background(), dev, nil, deployments, nil)
	require.NoError(t, err)
	assert.Len(t, assignments, 2)
}

func TestComputeAssignments_Update(t *testing.T) {
	versionID := uuid.New()
	newVariantID := uuid.New()

	orch := newTestOrchestrator(map[uuid.UUID][]VariantData{
		versionID: {{ID: newVariantID, VersionID: versionID, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "int8.bin", BlobSize: 1024, BlobSHA256: "sha"}},
	})

	dev := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 4096, HasGPU: true, Labels: map[string]string{}}
	loaded := []LoadedModelInfo{{ModelID: versionID.String(), VariantID: "old-variant-id"}}

	assignments, err := orch.ComputeAssignments(context.Background(), dev, nil, []DeploymentData{
		{ID: uuid.New(), ModelVersionID: versionID, Strategy: "ALL_AT_ONCE"},
	}, loaded)
	require.NoError(t, err)
	require.Len(t, assignments, 2) // UPDATE new + UNLOAD old

	var actions []string
	for _, a := range assignments {
		actions = append(actions, a.Action)
	}
	assert.Contains(t, actions, "UPDATE")
	assert.Contains(t, actions, "UNLOAD")
}

// Silence linter about unused imports
var _ = fmt.Sprintf

// Regression: a deployment created with target_selector {"region":"eu"} was
// delivered to every device because the flat map was decoded into the
// structured selector and silently became empty.
func TestComputeAssignments_LabelTargetingFromStoredSelector(t *testing.T) {
	versionID := uuid.New()
	orch := newTestOrchestrator(map[uuid.UUID][]VariantData{
		versionID: {{ID: uuid.New(), VersionID: versionID, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "int8.bin", BlobSize: 1024, BlobSHA256: "sha"}},
	})

	selector, err := ParseTargetSelector([]byte(`{"region":"eu"}`))
	require.NoError(t, err)
	deployments := []DeploymentData{{ID: uuid.New(), ModelVersionID: versionID, Strategy: "ALL_AT_ONCE", TargetSelector: selector}}

	euDevice := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 8192, HasGPU: true, Labels: map[string]string{"region": "eu"}}
	usDevice := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 8192, HasGPU: true, Labels: map[string]string{"region": "us"}}

	euAssignments, err := orch.ComputeAssignments(context.Background(), euDevice, nil, deployments, nil)
	require.NoError(t, err)
	require.Len(t, euAssignments, 1)
	assert.Equal(t, "LOAD", euAssignments[0].Action)

	usAssignments, err := orch.ComputeAssignments(context.Background(), usDevice, nil, deployments, nil)
	require.NoError(t, err)
	assert.Empty(t, usAssignments, "region=us device must not receive a region=eu deployment")
}

func TestComputeAssignments_ModelAndVersionIDs(t *testing.T) {
	modelID, v1, v2 := uuid.New(), uuid.New(), uuid.New()
	v2Variant := uuid.New()
	orch := newTestOrchestrator(map[uuid.UUID][]VariantData{
		v2: {{ID: v2Variant, VersionID: v2, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "v2.bin", BlobSize: 1024, BlobSHA256: "sha-v2"}},
	})
	dev := &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 4096, Labels: map[string]string{}}
	deps := []DeploymentData{{ID: uuid.New(), ModelID: modelID, ModelVersionID: v2, ModelName: "clf", VersionString: "2.0.0", Strategy: "ALL_AT_ONCE"}}

	// Fresh device: LOAD, with the model ID and the version ID in separate fields.
	got, err := orch.ComputeAssignments(context.Background(), dev, nil, deps, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, modelID.String(), got[0].ModelID)
	assert.Equal(t, v2.String(), got[0].VersionID)
	assert.Equal(t, "2.0.0", got[0].Version)
	assert.Equal(t, "LOAD", got[0].Action)

	// Device runs v1 of the same model and reports the model ID: UPDATE to v2, and v1 is unloaded.
	loaded := []LoadedModelInfo{{ModelID: modelID.String(), VariantID: uuid.NewString(), Version: "1.0.0"}}
	got, err = orch.ComputeAssignments(context.Background(), dev, nil, deps, loaded)
	require.NoError(t, err)
	actions := map[string]string{}
	for _, a := range got {
		actions[a.Action] = a.VariantID
	}
	assert.Equal(t, v2Variant.String(), actions["UPDATE"])
	assert.Equal(t, loaded[0].VariantID, actions["UNLOAD"])

	// Older SDKs echoed the version ID as model_id: still detected as the same model.
	legacy := []LoadedModelInfo{{ModelID: v1.String(), VariantID: uuid.NewString()}}
	deps[0].ModelVersionID = v1
	orch2 := newTestOrchestrator(map[uuid.UUID][]VariantData{
		v1: {{ID: uuid.New(), VersionID: v1, Quantization: "INT8", Runtime: "TFLITE", BlobKey: "v1.bin", BlobSize: 1024, BlobSHA256: "sha-v1"}},
	})
	got, err = orch2.ComputeAssignments(context.Background(), dev, nil, deps, legacy)
	require.NoError(t, err)
	assert.Equal(t, "UPDATE", got[0].Action)
}

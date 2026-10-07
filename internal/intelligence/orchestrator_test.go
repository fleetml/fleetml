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

package intelligence

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fp32Variant() CandidateVariant {
	return CandidateVariant{
		VariantID: uuid.New(), Quantization: "FP32", Runtime: "TFLITE", BlobSize: 100 * 1024 * 1024,
		Requirements: &VariantRequirements{MinRAMMB: 8192, Runtime: "TFLITE", MinScore: 60, Quantization: "FP32", Priority: 10, ModelSizeBytes: 100 * 1024 * 1024},
	}
}

func int8Variant() CandidateVariant {
	return CandidateVariant{
		VariantID: uuid.New(), Quantization: "INT8", Runtime: "TFLITE", BlobSize: 25 * 1024 * 1024,
		Requirements: &VariantRequirements{MinRAMMB: 1024, Runtime: "TFLITE", MinScore: 20, Quantization: "INT8", Priority: 30, ModelSizeBytes: 25 * 1024 * 1024},
	}
}

func int4Variant() CandidateVariant {
	return CandidateVariant{
		VariantID: uuid.New(), Quantization: "INT4", Runtime: "TFLITE", BlobSize: 12 * 1024 * 1024,
		Requirements: &VariantRequirements{MinRAMMB: 256, Runtime: "TFLITE", MinScore: 10, Quantization: "INT4", Priority: 40, ModelSizeBytes: 12 * 1024 * 1024},
	}
}

func flagshipProfile() *DeviceProfile {
	return &DeviceProfile{
		DeviceID: uuid.New(), DeviceClass: "android-flagship", OS: "ANDROID",
		Memory:       MemoryProfile{TotalRAMMB: 12288},
		Accelerators: AcceleratorProfile{HasNPU: true, HasGPU: true, NPUTflops: 45, SupportedRuntimes: []string{"TFLITE", "ONNX", "GGUF"}},
		Score:        85,
	}
}

func midrangeProfile() *DeviceProfile {
	return &DeviceProfile{
		DeviceID: uuid.New(), DeviceClass: "android-midrange", OS: "ANDROID",
		Memory:       MemoryProfile{TotalRAMMB: 4096},
		Accelerators: AcceleratorProfile{HasGPU: true, SupportedRuntimes: []string{"TFLITE", "ONNX", "GGUF"}},
		Score:        35,
	}
}

func lowendProfile() *DeviceProfile {
	return &DeviceProfile{
		DeviceID: uuid.New(), DeviceClass: "android-lowend", OS: "ANDROID",
		Memory:       MemoryProfile{TotalRAMMB: 2048},
		Accelerators: AcceleratorProfile{SupportedRuntimes: []string{"TFLITE", "ONNX", "GGUF"}},
		Score:        15,
	}
}

func TestSelect_FlagshipGetsFP32(t *testing.T) {
	s := NewSelector(nil)
	result := s.Select(flagshipProfile(), []CandidateVariant{fp32Variant(), int8Variant()})
	require.NotNil(t, result)
	assert.Equal(t, "FP32", result.Quantization)
	assert.Greater(t, result.Score, 0)
}

func TestSelect_MidrangeGetsINT8(t *testing.T) {
	s := NewSelector(nil)
	result := s.Select(midrangeProfile(), []CandidateVariant{fp32Variant(), int8Variant()})
	require.NotNil(t, result)
	assert.Equal(t, "INT8", result.Quantization)
}

func TestSelect_LowendGetsINT4(t *testing.T) {
	s := NewSelector(nil)
	// Lowend (score=15) can't run FP32 (minScore=60) or INT8 (minScore=20), but can run INT4 (minScore=10)
	result := s.Select(lowendProfile(), []CandidateVariant{fp32Variant(), int8Variant(), int4Variant()})
	require.NotNil(t, result)
	assert.Equal(t, "INT4", result.Quantization)
}

func TestSelect_LowendCantRunHighReqs(t *testing.T) {
	s := NewSelector(nil)
	// Lowend (score=15) can't run FP32 (minScore=60) or INT8 (minScore=20)
	result := s.Select(lowendProfile(), []CandidateVariant{fp32Variant(), int8Variant()})
	assert.Nil(t, result)
}

func TestSelect_NoCandidates(t *testing.T) {
	s := NewSelector(nil)
	result := s.Select(flagshipProfile(), nil)
	assert.Nil(t, result)
}

func TestSelect_NoCompatibleVariants(t *testing.T) {
	s := NewSelector(nil)
	// COREML variant on an Android device
	v := CandidateVariant{
		VariantID: uuid.New(), Quantization: "FP32", Runtime: "COREML",
		Requirements: &VariantRequirements{Runtime: "COREML", MinRAMMB: 1024},
	}
	result := s.Select(flagshipProfile(), []CandidateVariant{v})
	assert.Nil(t, result)
}

func TestSelect_SingleCandidate(t *testing.T) {
	s := NewSelector(nil)
	result := s.Select(flagshipProfile(), []CandidateVariant{int8Variant()})
	require.NotNil(t, result)
	assert.Equal(t, "INT8", result.Quantization)
}

func TestSelect_FiltersByRAM(t *testing.T) {
	s := NewSelector(nil)
	// FP32 needs 8GB, lowend has 2GB
	result := s.Select(lowendProfile(), []CandidateVariant{fp32Variant()})
	assert.Nil(t, result)
}

func TestSelect_FiltersByScore(t *testing.T) {
	s := NewSelector(nil)
	// FP32 needs score 60, midrange has 35
	result := s.Select(midrangeProfile(), []CandidateVariant{fp32Variant()})
	assert.Nil(t, result)
}

func TestSelect_FiltersByNPU(t *testing.T) {
	s := NewSelector(nil)
	npuVariant := CandidateVariant{
		VariantID: uuid.New(), Quantization: "INT8", Runtime: "TFLITE",
		Requirements: &VariantRequirements{Runtime: "TFLITE", RequiresNPU: true, MinRAMMB: 512},
	}
	// Midrange has no NPU
	result := s.Select(midrangeProfile(), []CandidateVariant{npuVariant})
	assert.Nil(t, result)

	// Flagship has NPU
	result = s.Select(flagshipProfile(), []CandidateVariant{npuVariant})
	require.NotNil(t, result)
}

func TestSelect_FiltersByStorage(t *testing.T) {
	s := NewSelector(nil)
	profile := flagshipProfile()
	profile.Memory.StorageFreeMB = 10 // Only 10MB free

	largeVariant := CandidateVariant{
		VariantID: uuid.New(), Quantization: "FP32", Runtime: "TFLITE",
		Requirements: &VariantRequirements{Runtime: "TFLITE", MinRAMMB: 1024, ModelSizeBytes: 100 * 1024 * 1024},
	}
	result := s.Select(profile, []CandidateVariant{largeVariant})
	assert.Nil(t, result)
}

func TestSelect_ThreeTierSelection(t *testing.T) {
	s := NewSelector(nil)
	candidates := []CandidateVariant{fp32Variant(), int8Variant(), int4Variant()}

	// Flagship: can run all, should pick based on best match score
	result := s.Select(flagshipProfile(), candidates)
	require.NotNil(t, result)

	// Midrange: can only run INT8 and INT4
	result = s.Select(midrangeProfile(), candidates)
	require.NotNil(t, result)
	assert.NotEqual(t, "FP32", result.Quantization)

	// Lowend: can only run INT8 and INT4
	result = s.Select(lowendProfile(), candidates)
	require.NotNil(t, result)
}

func TestSelect_NilRequirements(t *testing.T) {
	s := NewSelector(nil)
	v := CandidateVariant{VariantID: uuid.New(), Quantization: "FP32", Runtime: "TFLITE"}
	result := s.Select(flagshipProfile(), []CandidateVariant{v})
	require.NotNil(t, result)
}

func TestSelectWithDetails_ReturnsAllCandidates(t *testing.T) {
	s := NewSelector(nil)
	candidates := []CandidateVariant{fp32Variant(), int8Variant()}

	result, details := s.SelectWithDetails(midrangeProfile(), candidates)
	require.NotNil(t, result)
	assert.Len(t, details, 2)

	// One should be incompatible (FP32), one compatible (INT8)
	var incompatible, compatible int
	for _, d := range details {
		if d.Compatible {
			compatible++
		} else {
			incompatible++
		}
	}
	assert.Equal(t, 1, incompatible)
	assert.Equal(t, 1, compatible)
}

func TestSelect_ReasonIncludesClass(t *testing.T) {
	s := NewSelector(nil)
	result := s.Select(flagshipProfile(), []CandidateVariant{int8Variant()})
	require.NotNil(t, result)
	assert.Contains(t, result.Reason, "android-flagship")
}

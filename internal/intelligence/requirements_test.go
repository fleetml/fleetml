package intelligence

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRequirements_Defaults(t *testing.T) {
	tests := []struct {
		name         string
		quantization string
		wantMinRAM   int
		wantMinScore int
		wantPriority int
	}{
		{"FP32 defaults", "FP32", 2048, 60, 10},
		{"FP16 defaults", "FP16", 1024, 40, 20},
		{"INT8 defaults", "INT8", 512, 20, 30},
		{"INT4 defaults", "INT4", 256, 10, 40},
		{"DYNAMIC defaults", "DYNAMIC", 512, 15, 35},
		{"unknown defaults to DYNAMIC", "UNKNOWN", 512, 15, 35},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ParseRequirements(nil, tt.quantization, "TFLITE", 1024)
			assert.Equal(t, tt.wantMinRAM, req.MinRAMMB)
			assert.Equal(t, tt.wantMinScore, req.MinScore)
			assert.Equal(t, tt.wantPriority, req.Priority)
			assert.Equal(t, "TFLITE", req.Runtime)
			assert.Equal(t, int64(1024), req.ModelSizeBytes)
		})
	}
}

func TestParseRequirements_FromMetadata(t *testing.T) {
	meta := map[string]interface{}{
		"fleetml_requirements": map[string]interface{}{
			"min_ram_mb":   float64(4096),
			"runtime":      "ONNX",
			"min_score":    float64(50),
			"requires_npu": true,
			"requires_gpu": false,
		},
	}

	req := ParseRequirements(meta, "FP32", "TFLITE", 2048)
	assert.Equal(t, 4096, req.MinRAMMB)
	assert.Equal(t, "ONNX", req.Runtime)
	assert.Equal(t, 50, req.MinScore)
	assert.True(t, req.RequiresNPU)
	assert.False(t, req.RequiresGPU)
	assert.Equal(t, 10, req.Priority) // FP32 priority
}

func TestParseRequirements_PartialMetadata(t *testing.T) {
	meta := map[string]interface{}{
		"fleetml_requirements": map[string]interface{}{
			"min_ram_mb": float64(1024),
		},
	}

	req := ParseRequirements(meta, "INT8", "TFLITE", 512)
	assert.Equal(t, 1024, req.MinRAMMB) // from metadata
	assert.Equal(t, 20, req.MinScore)   // default for INT8
	assert.Equal(t, "TFLITE", req.Runtime)
}

func TestParseRequirements_NoRequirementsKey(t *testing.T) {
	meta := map[string]interface{}{
		"some_other_key": "value",
	}
	req := ParseRequirements(meta, "FP16", "ONNX", 4096)
	assert.Equal(t, 1024, req.MinRAMMB)
	assert.Equal(t, "ONNX", req.Runtime)
}

func TestParseRequirements_InvalidType(t *testing.T) {
	meta := map[string]interface{}{
		"fleetml_requirements": "not a map",
	}
	req := ParseRequirements(meta, "INT8", "TFLITE", 512)
	assert.Equal(t, 512, req.MinRAMMB) // falls back to default
}

func TestParseRequirements_NilMetadata(t *testing.T) {
	req := ParseRequirements(nil, "FP32", "COREML", 0)
	assert.Equal(t, 2048, req.MinRAMMB)
	assert.Equal(t, "COREML", req.Runtime)
	assert.Equal(t, "FP32", req.Quantization)
}

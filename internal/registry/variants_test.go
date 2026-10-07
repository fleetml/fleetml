package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func floatPtr(f float64) *float64 { return &f }

func TestSelectVariant(t *testing.T) {
	variants := []*ModelVariant{
		{ID: "fp32", Quantization: "FP32", TargetRuntime: "TFLITE", MinRAMMB: 2048},
		{ID: "fp16", Quantization: "FP16", TargetRuntime: "TFLITE", MinRAMMB: 1024},
		{ID: "int8", Quantization: "INT8", TargetRuntime: "TFLITE", MinRAMMB: 512},
		{ID: "int4", Quantization: "INT4", TargetRuntime: "TFLITE", MinRAMMB: 256},
		{ID: "onnx-fp16", Quantization: "FP16", TargetRuntime: "ONNX", MinRAMMB: 1024},
	}

	tests := []struct {
		name    string
		device  DeviceCapabilities
		wantID  string
		wantErr bool
	}{
		{
			name:   "high-end device gets FP32",
			device: DeviceCapabilities{RAMMB: 4096, Runtime: "TFLITE"},
			wantID: "fp32",
		},
		{
			name:   "mid device gets FP16",
			device: DeviceCapabilities{RAMMB: 1500, Runtime: "TFLITE"},
			wantID: "fp16",
		},
		{
			name:   "low device gets INT8",
			device: DeviceCapabilities{RAMMB: 600, Runtime: "TFLITE"},
			wantID: "int8",
		},
		{
			name:   "very low device gets INT4",
			device: DeviceCapabilities{RAMMB: 300, Runtime: "TFLITE"},
			wantID: "int4",
		},
		{
			name:   "onnx device gets onnx variant",
			device: DeviceCapabilities{RAMMB: 4096, Runtime: "ONNX"},
			wantID: "onnx-fp16",
		},
		{
			name:    "no matching runtime",
			device:  DeviceCapabilities{RAMMB: 4096, Runtime: "COREML"},
			wantErr: true,
		},
		{
			name:    "too little ram",
			device:  DeviceCapabilities{RAMMB: 100, Runtime: "TFLITE"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectVariant(variants, tt.device)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ID)
		})
	}
}

func TestSelectVariant_NPURequired(t *testing.T) {
	npu := floatPtr(4.0)
	variants := []*ModelVariant{
		{ID: "npu-variant", Quantization: "INT8", TargetRuntime: "TFLITE", MinRAMMB: 512, MinNPUTflops: npu},
	}

	_, err := SelectVariant(variants, DeviceCapabilities{RAMMB: 1024, Runtime: "TFLITE", HasNPU: false})
	assert.Error(t, err)

	got, err := SelectVariant(variants, DeviceCapabilities{RAMMB: 1024, Runtime: "TFLITE", HasNPU: true, NPUTflops: 8.0})
	require.NoError(t, err)
	assert.Equal(t, "npu-variant", got.ID)
}

func TestSelectVariant_Empty(t *testing.T) {
	_, err := SelectVariant(nil, DeviceCapabilities{})
	assert.Error(t, err)
}

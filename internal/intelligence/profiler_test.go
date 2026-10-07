package intelligence

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/fleetml/fleetml/internal/device"
)

func TestBuildProfile(t *testing.T) {
	p := NewProfiler(nil)

	tests := []struct {
		name          string
		device        *device.Device
		health        *HealthSnapshot
		wantClass     string
		wantScoreMin  int
		wantScoreMax  int
		wantRuntimes  []string
	}{
		{
			name:         "android flagship",
			device:       &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 12288, HasNPU: true, NPUTflops: 45, HasGPU: true, Chipset: "snapdragon-8-gen-3"},
			wantClass:    "android-flagship",
			wantScoreMin: 50,
			wantScoreMax: 100,
			wantRuntimes: []string{"TFLITE", "ONNX", "GGUF"},
		},
		{
			name:         "android midrange",
			device:       &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 6144, HasNPU: false, HasGPU: true, Chipset: "snapdragon-695"},
			wantClass:    "android-midrange",
			wantScoreMin: 20,
			wantScoreMax: 70,
			wantRuntimes: []string{"TFLITE", "ONNX", "GGUF"},
		},
		{
			name:         "android lowend",
			device:       &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 2048, HasNPU: false, HasGPU: false},
			wantClass:    "android-lowend",
			wantScoreMin: 0,
			wantScoreMax: 40,
		},
		{
			name:         "ios pro",
			device:       &device.Device{ID: uuid.New(), OS: "IOS", RAMMB: 8192, HasNPU: true, NPUTflops: 15, Chipset: "a17-pro"},
			wantClass:    "ios-pro",
			wantScoreMin: 40,
			wantScoreMax: 100,
			wantRuntimes: []string{"COREML", "TFLITE", "GGUF"},
		},
		{
			name:         "ios standard",
			device:       &device.Device{ID: uuid.New(), OS: "IOS", RAMMB: 4096, HasNPU: false, Chipset: "a15"},
			wantClass:    "ios-standard",
			wantScoreMin: 10,
			wantScoreMax: 60,
		},
		{
			name:         "linux jetson orin",
			device:       &device.Device{ID: uuid.New(), OS: "LINUX", RAMMB: 16384, HasNPU: true, NPUTflops: 100, Chipset: "nvidia-jetson-orin"},
			wantClass:    "linux-jetson-orin",
			wantScoreMin: 60,
			wantScoreMax: 100,
		},
		{
			name:         "linux jetson nano",
			device:       &device.Device{ID: uuid.New(), OS: "LINUX", RAMMB: 4096, HasNPU: true, NPUTflops: 5, Chipset: "jetson-nano"},
			wantClass:    "linux-jetson-nano",
			wantScoreMin: 10,
			wantScoreMax: 60,
		},
		{
			name:         "linux rpi4",
			device:       &device.Device{ID: uuid.New(), OS: "LINUX", RAMMB: 4096, Chipset: "raspberry-pi-4"},
			wantClass:    "linux-rpi4",
			wantScoreMin: 5,
			wantScoreMax: 50,
		},
		{
			name:         "linux rpi5",
			device:       &device.Device{ID: uuid.New(), OS: "LINUX", RAMMB: 8192, Chipset: "rpi-5"},
			wantClass:    "linux-rpi5",
			wantScoreMin: 10,
			wantScoreMax: 60,
		},
		{
			name:         "linux server",
			device:       &device.Device{ID: uuid.New(), OS: "LINUX", RAMMB: 65536, HasGPU: true, Chipset: "intel-xeon"},
			wantClass:    "linux-server",
			wantScoreMin: 30,
			wantScoreMax: 80,
		},
		{
			name:         "windows gpu",
			device:       &device.Device{ID: uuid.New(), OS: "WINDOWS", RAMMB: 16384, HasGPU: true, Chipset: "intel-i7"},
			wantClass:    "windows-gpu",
			wantScoreMin: 20,
			wantScoreMax: 80,
			wantRuntimes: []string{"ONNX", "TFLITE", "GGUF"},
		},
		{
			name:         "windows standard",
			device:       &device.Device{ID: uuid.New(), OS: "WINDOWS", RAMMB: 8192, HasGPU: false, Chipset: "arm-snapdragon"},
			wantClass:    "windows-standard",
			wantScoreMin: 10,
			wantScoreMax: 60,
		},
		{
			name:         "with health data",
			device:       &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 8192, HasNPU: true, NPUTflops: 20},
			health:       &HealthSnapshot{AvailableRAMMB: 4096, StorageFreeMB: 10000, NetworkType: "wifi"},
			wantClass:    "android-flagship",
			wantScoreMin: 40,
			wantScoreMax: 100,
		},
		{
			name:         "zero RAM",
			device:       &device.Device{ID: uuid.New(), OS: "ANDROID", RAMMB: 0},
			wantClass:    "android-lowend",
			wantScoreMin: 0,
			wantScoreMax: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := p.BuildProfile(tt.device, tt.health)
			assert.Equal(t, tt.wantClass, profile.DeviceClass)
			assert.GreaterOrEqual(t, profile.Score, tt.wantScoreMin, "score too low")
			assert.LessOrEqual(t, profile.Score, tt.wantScoreMax, "score too high")
			if tt.wantRuntimes != nil {
				assert.Equal(t, tt.wantRuntimes, profile.Accelerators.SupportedRuntimes)
			}
			if tt.health != nil {
				assert.Equal(t, tt.health.AvailableRAMMB, profile.Memory.AvailableRAMMB)
				assert.Equal(t, tt.health.NetworkType, profile.Connectivity.NetworkType)
			}
		})
	}
}

func TestClassifyDevice_AllOSTypes(t *testing.T) {
	p := NewProfiler(nil)

	tests := []struct {
		os      string
		chipset string
		ramMB   int
		hasNPU  bool
		hasGPU  bool
		want    string
	}{
		{"ANDROID", "", 12288, true, true, "android-flagship"},
		{"ANDROID", "", 6144, false, true, "android-midrange"},
		{"ANDROID", "", 2048, false, false, "android-lowend"},
		{"IOS", "", 8192, true, false, "ios-pro"},
		{"IOS", "", 4096, false, false, "ios-standard"},
		{"LINUX", "jetson-orin", 16384, true, true, "linux-jetson-orin"},
		{"LINUX", "jetson-nano", 4096, true, false, "linux-jetson-nano"},
		{"LINUX", "raspberry-pi-4", 4096, false, false, "linux-rpi4"},
		{"LINUX", "rpi-5", 8192, false, false, "linux-rpi5"},
		{"LINUX", "intel-xeon", 65536, false, false, "linux-server"},
		{"LINUX", "arm-cortex", 2048, false, false, "linux-standard"},
		{"WINDOWS", "", 16384, false, true, "windows-gpu"},
		{"WINDOWS", "", 8192, false, false, "windows-standard"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			d := &device.Device{OS: tt.os, Chipset: tt.chipset, RAMMB: tt.ramMB, HasNPU: tt.hasNPU, HasGPU: tt.hasGPU}
			assert.Equal(t, tt.want, p.ClassifyDevice(d))
		})
	}
}

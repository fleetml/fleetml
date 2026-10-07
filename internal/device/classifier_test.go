package device

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyDevice(t *testing.T) {
	tests := []struct {
		name     string
		device   *Device
		expected string
	}{
		{
			name:     "android flagship",
			device:   &Device{OS: "ANDROID", RAMMB: 12288, HasNPU: true, Chipset: "snapdragon-8-gen-3"},
			expected: "android-flagship",
		},
		{
			name:     "android midrange",
			device:   &Device{OS: "ANDROID", RAMMB: 6144, HasNPU: false, Chipset: "snapdragon-695"},
			expected: "android-midrange",
		},
		{
			name:     "android lowend",
			device:   &Device{OS: "ANDROID", RAMMB: 2048, HasNPU: false, Chipset: "helio-g25"},
			expected: "android-lowend",
		},
		{
			name:     "ios flagship",
			device:   &Device{OS: "IOS", RAMMB: 8192, HasNPU: true, Chipset: "a17-pro"},
			expected: "ios-flagship",
		},
		{
			name:     "ios midrange",
			device:   &Device{OS: "IOS", RAMMB: 4096, HasNPU: false, Chipset: "a15"},
			expected: "ios-midrange",
		},
		{
			name:     "linux jetson",
			device:   &Device{OS: "LINUX", RAMMB: 16384, HasNPU: true, Chipset: "nvidia-jetson-orin"},
			expected: "linux-jetson",
		},
		{
			name:     "linux rpi",
			device:   &Device{OS: "LINUX", RAMMB: 4096, HasNPU: false, Chipset: "raspberry-pi-4"},
			expected: "linux-rpi",
		},
		{
			name:     "linux rpi alt",
			device:   &Device{OS: "LINUX", RAMMB: 8192, HasNPU: false, Chipset: "rpi-5"},
			expected: "linux-rpi",
		},
		{
			name:     "linux standard",
			device:   &Device{OS: "LINUX", RAMMB: 32768, HasNPU: false, Chipset: "intel-i7"},
			expected: "linux-standard",
		},
		{
			name:     "windows flagship",
			device:   &Device{OS: "WINDOWS", RAMMB: 16384, HasNPU: true, Chipset: "snapdragon-x-elite"},
			expected: "windows-flagship",
		},
		{
			name:     "windows midrange",
			device:   &Device{OS: "WINDOWS", RAMMB: 8192, HasNPU: false, Chipset: "intel-i5"},
			expected: "windows-midrange",
		},
		{
			name:     "8GB RAM with NPU is flagship",
			device:   &Device{OS: "ANDROID", RAMMB: 8192, HasNPU: true},
			expected: "android-flagship",
		},
		{
			name:     "8GB RAM without NPU is midrange",
			device:   &Device{OS: "ANDROID", RAMMB: 8192, HasNPU: false},
			expected: "android-midrange",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyDevice(tt.device)
			assert.Equal(t, tt.expected, result)
		})
	}
}

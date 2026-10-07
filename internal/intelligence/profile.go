package intelligence

import "github.com/google/uuid"

// DeviceProfile represents the full hardware capability assessment of a device.
type DeviceProfile struct {
	DeviceID     uuid.UUID
	DeviceClass  string
	OS           string
	Compute      ComputeProfile
	Memory       MemoryProfile
	Accelerators AcceleratorProfile
	Connectivity ConnectivityProfile
	Score        int // Overall capability score (0-100)
}

// ComputeProfile describes the CPU capabilities.
type ComputeProfile struct {
	CPUCores     int
	Architecture string // "arm64", "x86_64", "armv7"
	IntScore     int    // Integer ops score (0-100)
	FloatScore   int    // Float ops score (0-100)
}

// MemoryProfile describes memory and storage capabilities.
type MemoryProfile struct {
	TotalRAMMB     int
	AvailableRAMMB int
	StorageFreeMB  int
}

// AcceleratorProfile describes NPU/GPU capabilities.
type AcceleratorProfile struct {
	HasNPU            bool
	HasGPU            bool
	NPUTflops         float64
	GPUTflops         float64
	SupportedRuntimes []string
}

// ConnectivityProfile describes network conditions.
type ConnectivityProfile struct {
	NetworkType   string
	BandwidthMbps float64
	IsMetered     bool
}

// HealthSnapshot represents runtime health data from a heartbeat.
type HealthSnapshot struct {
	AvailableRAMMB int
	BatteryPercent float64
	StorageFreeMB  int
	NetworkType    string
	BandwidthMbps  float64
	IsMetered      bool
}

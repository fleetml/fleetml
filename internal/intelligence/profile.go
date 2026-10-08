package intelligence

import "github.com/google/uuid"

// DeviceProfile represents the full hardware capability assessment of a device.
type DeviceProfile struct {
	DeviceID     uuid.UUID           `json:"device_id"`
	DeviceClass  string              `json:"device_class"`
	OS           string              `json:"os"`
	Compute      ComputeProfile      `json:"compute"`
	Memory       MemoryProfile       `json:"memory"`
	Accelerators AcceleratorProfile  `json:"accelerators"`
	Connectivity ConnectivityProfile `json:"connectivity"`
	Score        int                 `json:"score"` // Overall capability score (0-100)
}

// ComputeProfile describes the CPU capabilities.
type ComputeProfile struct {
	CPUCores     int    `json:"cpu_cores"`
	Architecture string `json:"architecture"` // "arm64", "x86_64", "armv7"
	IntScore     int    `json:"int_score"`    // Integer ops score (0-100)
	FloatScore   int    `json:"float_score"`  // Float ops score (0-100)
}

// MemoryProfile describes memory and storage capabilities.
type MemoryProfile struct {
	TotalRAMMB     int `json:"total_ram_mb"`
	AvailableRAMMB int `json:"available_ram_mb"`
	StorageFreeMB  int `json:"storage_free_mb"`
}

// AcceleratorProfile describes NPU/GPU capabilities.
type AcceleratorProfile struct {
	HasNPU            bool     `json:"has_npu"`
	HasGPU            bool     `json:"has_gpu"`
	NPUTflops         float64  `json:"npu_tflops"`
	GPUTflops         float64  `json:"gpu_tflops"`
	SupportedRuntimes []string `json:"supported_runtimes"`
}

// ConnectivityProfile describes network conditions.
type ConnectivityProfile struct {
	NetworkType   string  `json:"network_type"`
	BandwidthMbps float64 `json:"bandwidth_mbps"`
	IsMetered     bool    `json:"is_metered"`
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

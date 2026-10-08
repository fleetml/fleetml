package intelligence

import (
	"log/slog"
	"strings"

	"github.com/fleetml/fleetml/internal/device"
)

const (
	computeWeight     = 0.3
	memoryWeight      = 0.3
	acceleratorWeight = 0.4
)

// Profiler builds DeviceProfiles from raw device data.
type Profiler struct {
	logger *slog.Logger
}

// NewProfiler creates a new Profiler.
func NewProfiler(logger *slog.Logger) *Profiler {
	return &Profiler{logger: logger}
}

// BuildProfile constructs a DeviceProfile from device registration data
// and optional runtime health data from the latest heartbeat.
func (p *Profiler) BuildProfile(d *device.Device, health *HealthSnapshot) *DeviceProfile {
	compute := buildComputeProfile(d)
	memory := buildMemoryProfile(d, health)
	accel := buildAcceleratorProfile(d)
	conn := buildConnectivityProfile(health)
	className := p.ClassifyDevice(d)

	computeScore := min100(compute.CPUCores*10 + archBonus(compute.Architecture))
	memoryScore := min100(d.RAMMB / 80)
	accelScore := acceleratorScore(d)

	overall := int(float64(computeScore)*computeWeight +
		float64(memoryScore)*memoryWeight +
		float64(accelScore)*acceleratorWeight)

	return &DeviceProfile{
		DeviceID:     d.ID,
		DeviceClass:  className,
		OS:           d.OS,
		Compute:      compute,
		Memory:       memory,
		Accelerators: accel,
		Connectivity: conn,
		Score:        overall,
	}
}

// ClassifyDevice returns an enhanced device class string.
func (p *Profiler) ClassifyDevice(d *device.Device) string {
	os := strings.ToLower(d.OS)
	chipset := strings.ToLower(d.Chipset)

	switch os {
	case "linux":
		return classifyLinux(chipset, d.RAMMB)
	case "ios":
		return classifyIOS(d.RAMMB, d.HasNPU)
	case "android":
		return "android-" + classifyTier(d.RAMMB, d.HasNPU)
	case "windows":
		if d.HasGPU {
			return "windows-gpu"
		}
		return "windows-standard"
	default:
		return "unknown-" + classifyTier(d.RAMMB, d.HasNPU)
	}
}

func classifyLinux(chipset string, ramMB int) string {
	switch {
	case strings.Contains(chipset, "jetson") && strings.Contains(chipset, "orin"):
		return "linux-jetson-orin"
	case strings.Contains(chipset, "jetson") && strings.Contains(chipset, "nano"):
		return "linux-jetson-nano"
	case strings.Contains(chipset, "jetson"):
		return "linux-jetson"
	case strings.Contains(chipset, "rpi-5") || strings.Contains(chipset, "raspberry-pi-5"):
		return "linux-rpi5"
	case strings.Contains(chipset, "rpi") || strings.Contains(chipset, "raspberry"):
		return "linux-rpi4"
	case ramMB >= 32768:
		return "linux-server"
	default:
		return "linux-standard"
	}
}

func classifyIOS(ramMB int, hasNPU bool) string {
	if ramMB >= 6144 && hasNPU {
		return "ios-pro"
	}
	return "ios-standard"
}

func classifyTier(ramMB int, hasNPU bool) string {
	if ramMB >= 8192 && hasNPU {
		return "flagship"
	}
	if ramMB >= 4096 {
		return "midrange"
	}
	return "lowend"
}

func buildComputeProfile(d *device.Device) ComputeProfile {
	arch := inferArchitecture(d.OS, d.Chipset)
	cores := inferCPUCores(d.OS)
	intScore := min100(cores*10 + archBonus(arch))
	floatScore := intScore
	if d.HasNPU || d.HasGPU {
		floatScore = min100(floatScore + 20)
	}
	return ComputeProfile{
		CPUCores:     cores,
		Architecture: arch,
		IntScore:     intScore,
		FloatScore:   floatScore,
	}
}

func buildMemoryProfile(d *device.Device, health *HealthSnapshot) MemoryProfile {
	mp := MemoryProfile{TotalRAMMB: d.RAMMB}
	if health != nil {
		mp.AvailableRAMMB = health.AvailableRAMMB
		mp.StorageFreeMB = health.StorageFreeMB
	}
	return mp
}

func buildAcceleratorProfile(d *device.Device) AcceleratorProfile {
	runtimes := inferSupportedRuntimes(d.OS)
	return AcceleratorProfile{
		HasNPU:            d.HasNPU,
		HasGPU:            d.HasGPU,
		NPUTflops:         d.NPUTflops,
		SupportedRuntimes: runtimes,
	}
}

func buildConnectivityProfile(health *HealthSnapshot) ConnectivityProfile {
	if health == nil {
		return ConnectivityProfile{NetworkType: "unknown"}
	}
	return ConnectivityProfile{
		NetworkType:   health.NetworkType,
		BandwidthMbps: health.BandwidthMbps,
		IsMetered:     health.IsMetered,
	}
}

func inferSupportedRuntimes(os string) []string {
	switch strings.ToUpper(os) {
	case "ANDROID":
		return []string{"TFLITE", "ONNX", "GGUF"}
	case "IOS":
		return []string{"COREML", "TFLITE", "GGUF"}
	case "LINUX":
		// The device agent validates any format with a configured command.
		return []string{"TFLITE", "ONNX", "GGUF", "CUSTOM"}
	case "WINDOWS":
		return []string{"ONNX", "TFLITE", "GGUF"}
	default:
		return []string{"CUSTOM"}
	}
}

func inferArchitecture(os, chipset string) string {
	chipLower := strings.ToLower(chipset)
	switch strings.ToUpper(os) {
	case "ANDROID":
		return "arm64"
	case "IOS":
		return "arm64"
	case "LINUX":
		if strings.Contains(chipLower, "jetson") || strings.Contains(chipLower, "rpi") || strings.Contains(chipLower, "raspberry") ||
			strings.Contains(chipLower, "arm64") || strings.Contains(chipLower, "aarch64") {
			return "arm64"
		}
		return "x86_64"
	case "WINDOWS":
		if strings.Contains(chipLower, "snapdragon") || strings.Contains(chipLower, "arm") {
			return "arm64"
		}
		return "x86_64"
	default:
		return "unknown"
	}
}

func inferCPUCores(os string) int {
	switch strings.ToUpper(os) {
	case "ANDROID":
		return 8
	case "IOS":
		return 6
	case "LINUX":
		return 4
	case "WINDOWS":
		return 8
	default:
		return 4
	}
}

func archBonus(arch string) int {
	if arch == "arm64" {
		return 20
	}
	return 0
}

func acceleratorScore(d *device.Device) int {
	if d.HasNPU {
		return min100(int(d.NPUTflops * 2))
	}
	if d.HasGPU {
		return 30
	}
	return 0
}

func min100(v int) int {
	if v > 100 {
		return 100
	}
	if v < 0 {
		return 0
	}
	return v
}

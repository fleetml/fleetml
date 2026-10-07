package device

import "strings"

// ClassifyDevice returns a device class name based on hardware capabilities.
func ClassifyDevice(d *Device) string {
	os := strings.ToLower(d.OS)
	chipset := strings.ToLower(d.Chipset)

	switch os {
	case "linux":
		if strings.Contains(chipset, "jetson") {
			return "linux-jetson"
		}
		if strings.Contains(chipset, "raspberry") || strings.Contains(chipset, "rpi") {
			return "linux-rpi"
		}
		return "linux-standard"
	case "android", "ios", "windows":
		return os + "-" + classifyTier(d.RAMMB, d.HasNPU)
	default:
		return "unknown-" + classifyTier(d.RAMMB, d.HasNPU)
	}
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

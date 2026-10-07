package registry

import "fmt"

type DeviceCapabilities struct {
	RAMMB     int
	HasNPU    bool
	HasGPU    bool
	NPUTflops float64
	Runtime   string
}

func SelectVariant(variants []*ModelVariant, device DeviceCapabilities) (*ModelVariant, error) {
	if len(variants) == 0 {
		return nil, fmt.Errorf("selecting variant: %w: no variants available", ErrNotFound)
	}

	var candidates []*ModelVariant
	for _, v := range variants {
		if v.TargetRuntime != device.Runtime {
			continue
		}
		if v.MinRAMMB > device.RAMMB {
			continue
		}
		if v.MinNPUTflops != nil && *v.MinNPUTflops > 0 {
			if !device.HasNPU || device.NPUTflops < *v.MinNPUTflops {
				continue
			}
		}
		candidates = append(candidates, v)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("selecting variant: %w: no compatible variant for device", ErrNotFound)
	}

	// Prefer highest quality that fits: FP32 > FP16 > INT8 > INT4 > DYNAMIC
	quantPriority := map[string]int{
		"FP32":    5,
		"FP16":    4,
		"INT8":    3,
		"INT4":    2,
		"DYNAMIC": 1,
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if quantPriority[c.Quantization] > quantPriority[best.Quantization] {
			best = c
		}
	}

	return best, nil
}

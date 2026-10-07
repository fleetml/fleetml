package intelligence

// VariantRequirements specifies what a model variant needs from a device.
type VariantRequirements struct {
	MinRAMMB       int    `json:"min_ram_mb"`
	Runtime        string `json:"runtime"`
	MinScore       int    `json:"min_score"`
	RequiresNPU    bool   `json:"requires_npu"`
	RequiresGPU    bool   `json:"requires_gpu"`
	ModelSizeBytes int64  `json:"model_size_bytes"`
	Quantization   string `json:"quantization"`
	Priority       int    `json:"priority"`
}

// quantization defaults: (min_ram, min_score, priority)
var quantDefaults = map[string][3]int{
	"FP32":    {2048, 60, 10},
	"FP16":    {1024, 40, 20},
	"INT8":    {512, 20, 30},
	"INT4":    {256, 10, 40},
	"DYNAMIC": {512, 15, 35},
}

// ParseRequirements extracts VariantRequirements from variant metadata.
// If metadata has a "fleetml_requirements" key, those values are used.
// Missing fields get defaults based on quantization.
func ParseRequirements(metadataJSON map[string]interface{}, quantization, runtime string, blobSize int64) *VariantRequirements {
	defaults := quantDefaults[quantization]
	if defaults == [3]int{} {
		defaults = quantDefaults["DYNAMIC"]
	}

	req := &VariantRequirements{
		MinRAMMB:       defaults[0],
		Runtime:        runtime,
		MinScore:       defaults[1],
		ModelSizeBytes: blobSize,
		Quantization:   quantization,
		Priority:       defaults[2],
	}

	if metadataJSON == nil {
		return req
	}

	raw, ok := metadataJSON["fleetml_requirements"]
	if !ok {
		return req
	}
	reqMap, ok := raw.(map[string]interface{})
	if !ok {
		return req
	}

	if v, ok := intFromMap(reqMap, "min_ram_mb"); ok {
		req.MinRAMMB = v
	}
	if v, ok := reqMap["runtime"].(string); ok && v != "" {
		req.Runtime = v
	}
	if v, ok := intFromMap(reqMap, "min_score"); ok {
		req.MinScore = v
	}
	if v, ok := reqMap["requires_npu"].(bool); ok {
		req.RequiresNPU = v
	}
	if v, ok := reqMap["requires_gpu"].(bool); ok {
		req.RequiresGPU = v
	}

	return req
}

func intFromMap(m map[string]interface{}, key string) (int, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

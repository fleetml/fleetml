package intelligence

// TargetSelector defines which devices a deployment targets.
type TargetSelector struct {
	DeviceClasses []string          `json:"device_classes,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	MinScore      int               `json:"min_score,omitempty"`
	OS            []string          `json:"os,omitempty"`
	Runtimes      []string          `json:"runtimes,omitempty"`
}

// Matches checks if a device profile matches this target selector.
// An empty selector matches ALL devices.
// All non-empty criteria are AND'd together.
func (ts *TargetSelector) Matches(profile *DeviceProfile, deviceLabels map[string]string) bool {
	if len(ts.DeviceClasses) > 0 {
		if !containsString(ts.DeviceClasses, profile.DeviceClass) {
			return false
		}
	}

	if len(ts.Labels) > 0 {
		for k, v := range ts.Labels {
			if deviceLabels[k] != v {
				return false
			}
		}
	}

	if ts.MinScore > 0 && profile.Score < ts.MinScore {
		return false
	}

	if len(ts.OS) > 0 {
		if !containsString(ts.OS, profile.OS) {
			return false
		}
	}

	if len(ts.Runtimes) > 0 {
		if !hasIntersection(ts.Runtimes, profile.Accelerators.SupportedRuntimes) {
			return false
		}
	}

	return true
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func hasIntersection(a, b []string) bool {
	set := make(map[string]bool, len(b))
	for _, v := range b {
		set[v] = true
	}
	for _, v := range a {
		if set[v] {
			return true
		}
	}
	return false
}

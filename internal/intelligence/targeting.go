package intelligence

import (
	"encoding/json"
	"fmt"
)

// TargetSelector defines which devices a deployment targets.
type TargetSelector struct {
	DeviceClasses []string          `json:"device_classes,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	MinScore      int               `json:"min_score,omitempty"`
	OS            []string          `json:"os,omitempty"`
	Runtimes      []string          `json:"runtimes,omitempty"`
}

// ParseTargetSelector decodes a stored deployment target_selector.
//
// Deployments are created with a flat label map such as {"region":"eu"};
// those keys are device labels. An object whose values are not all strings is
// decoded as a structured selector (device_classes, labels, min_score, ...).
func ParseTargetSelector(raw []byte) (TargetSelector, error) {
	var ts TargetSelector
	if len(raw) == 0 || string(raw) == "null" {
		return ts, nil
	}
	var flat map[string]string
	if err := json.Unmarshal(raw, &flat); err == nil {
		if len(flat) > 0 {
			ts.Labels = flat
		}
		return ts, nil
	}
	if err := json.Unmarshal(raw, &ts); err != nil {
		return TargetSelector{}, fmt.Errorf("parsing target_selector: %w", err)
	}
	return ts, nil
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

package intelligence

import (
	"fmt"
	"log/slog"
	"sort"

	"github.com/google/uuid"
)

// Selector picks the optimal model variant for a given device profile.
type Selector struct {
	logger *slog.Logger
}

// NewSelector creates a new Selector.
func NewSelector(logger *slog.Logger) *Selector {
	return &Selector{logger: logger}
}

// CandidateVariant combines variant metadata with parsed requirements.
type CandidateVariant struct {
	VariantID    uuid.UUID
	VersionID    uuid.UUID
	Quantization string
	Runtime      string
	BlobKey      string
	BlobSize     int64
	BlobSHA256   string
	Requirements *VariantRequirements
}

// SelectionResult represents the outcome of variant selection.
type SelectionResult struct {
	VariantID    uuid.UUID
	Quantization string
	Score        int
	Reason       string
}

// CandidateResult describes a candidate's compatibility for the preview endpoint.
type CandidateResult struct {
	VariantID    uuid.UUID `json:"variant_id"`
	Quantization string    `json:"quantization"`
	Compatible   bool      `json:"compatible"`
	Score        int       `json:"score,omitempty"`
	Reason       string    `json:"reason"`
}

// Select chooses the best variant from candidates for the given device profile.
// Returns nil if no variant is compatible.
func (s *Selector) Select(profile *DeviceProfile, candidates []CandidateVariant) *SelectionResult {
	if len(candidates) == 0 {
		return nil
	}

	type scored struct {
		candidate CandidateVariant
		score     float64
	}

	var compatible []scored
	for _, c := range candidates {
		if reason := checkCompatibility(profile, c); reason != "" {
			continue
		}
		ms := matchScore(profile, c)
		compatible = append(compatible, scored{candidate: c, score: ms})
	}

	if len(compatible) == 0 {
		return nil
	}

	sort.Slice(compatible, func(i, j int) bool {
		return compatible[i].score > compatible[j].score
	})

	best := compatible[0]
	scoreInt := int(best.score * 100)
	reason := fmt.Sprintf("%s variant selected for %s (score %d, RAM headroom %.0f%%)",
		best.candidate.Quantization, profile.DeviceClass, scoreInt,
		memoryHeadroomPct(profile, best.candidate))

	return &SelectionResult{
		VariantID:    best.candidate.VariantID,
		Quantization: best.candidate.Quantization,
		Score:        scoreInt,
		Reason:       reason,
	}
}

// SelectWithDetails returns both the selection result and details for all candidates.
func (s *Selector) SelectWithDetails(profile *DeviceProfile, candidates []CandidateVariant) (*SelectionResult, []CandidateResult) {
	details := make([]CandidateResult, 0, len(candidates))

	type scored struct {
		candidate CandidateVariant
		score     float64
	}
	var compatible []scored

	for _, c := range candidates {
		reason := checkCompatibility(profile, c)
		if reason != "" {
			details = append(details, CandidateResult{
				VariantID: c.VariantID, Quantization: c.Quantization,
				Compatible: false, Reason: reason,
			})
			continue
		}
		ms := matchScore(profile, c)
		compatible = append(compatible, scored{candidate: c, score: ms})
		details = append(details, CandidateResult{
			VariantID: c.VariantID, Quantization: c.Quantization,
			Compatible: true, Score: int(ms * 100), Reason: "compatible",
		})
	}

	if len(compatible) == 0 {
		return nil, details
	}

	sort.Slice(compatible, func(i, j int) bool {
		return compatible[i].score > compatible[j].score
	})

	best := compatible[0]
	scoreInt := int(best.score * 100)
	reason := fmt.Sprintf("%s variant selected for %s (score %d, RAM headroom %.0f%%)",
		best.candidate.Quantization, profile.DeviceClass, scoreInt,
		memoryHeadroomPct(profile, best.candidate))

	return &SelectionResult{
		VariantID:    best.candidate.VariantID,
		Quantization: best.candidate.Quantization,
		Score:        scoreInt,
		Reason:       reason,
	}, details
}

func checkCompatibility(profile *DeviceProfile, c CandidateVariant) string {
	req := c.Requirements
	if req == nil {
		return ""
	}

	if req.Runtime != "" && !containsString(profile.Accelerators.SupportedRuntimes, req.Runtime) {
		return fmt.Sprintf("runtime %s not supported (device supports %v)", req.Runtime, profile.Accelerators.SupportedRuntimes)
	}
	if req.MinRAMMB > 0 && profile.Memory.TotalRAMMB < req.MinRAMMB {
		return fmt.Sprintf("requires %dMB RAM, device has %dMB", req.MinRAMMB, profile.Memory.TotalRAMMB)
	}
	if req.MinScore > 0 && profile.Score < req.MinScore {
		return fmt.Sprintf("requires score %d, device has %d", req.MinScore, profile.Score)
	}
	if req.RequiresNPU && !profile.Accelerators.HasNPU {
		return "requires NPU, device has none"
	}
	if req.RequiresGPU && !profile.Accelerators.HasGPU {
		return "requires GPU, device has none"
	}
	if req.ModelSizeBytes > 0 && profile.Memory.StorageFreeMB > 0 {
		requiredMB := req.ModelSizeBytes / (1024 * 1024)
		if requiredMB > int64(profile.Memory.StorageFreeMB) {
			return fmt.Sprintf("model %dMB exceeds free storage %dMB", requiredMB, profile.Memory.StorageFreeMB)
		}
	}
	return ""
}

func matchScore(profile *DeviceProfile, c CandidateVariant) float64 {
	req := c.Requirements
	if req == nil {
		return 0.5
	}

	// Quality score: prefer highest-quality variant the device can run.
	// FP32(priority=10) → quality=1.0, FP16(priority=20) → 0.75, INT8(priority=30) → 0.5, INT4(priority=40) → 0.25
	qualityScore := 1.0 - (float64(req.Priority) / 50.0)
	if qualityScore < 0 {
		qualityScore = 0
	}

	// Feasibility: how comfortably can the device run this variant (RAM headroom)
	memHeadroom := memoryHeadroomPct(profile, c) / 100.0

	// Score headroom: how much the device exceeds minimum score requirement
	scoreHeadroom := 0.0
	if req.MinScore > 0 && profile.Score > req.MinScore {
		scoreHeadroom = float64(profile.Score-req.MinScore) / 100.0
	}

	// Quality-first: highest quality variant that fits comfortably.
	return qualityScore*0.7 + memHeadroom*0.15 + scoreHeadroom*0.15
}

func memoryHeadroomPct(profile *DeviceProfile, c CandidateVariant) float64 {
	if c.Requirements == nil || profile.Memory.TotalRAMMB == 0 {
		return 50.0
	}
	headroom := float64(profile.Memory.TotalRAMMB-c.Requirements.MinRAMMB) / float64(profile.Memory.TotalRAMMB) * 100.0
	if headroom < 0 {
		return 0
	}
	if headroom > 100 {
		return 100
	}
	return headroom
}

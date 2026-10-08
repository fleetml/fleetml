package experiment

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/device"
	"github.com/fleetml/fleetml/internal/intelligence"
)

// ArmID is the stable identifier of one experiment arm. Devices receive it in
// the heartbeat and send it back as experiment_arm_id in telemetry, so each
// event is attributed to the arm that produced it.
func ArmID(experimentID uuid.UUID, cohort string) uuid.UUID {
	return uuid.NewSHA1(experimentID, []byte(cohort))
}

// VersionResolver picks the variant of a model version a device should run.
type VersionResolver interface {
	ResolveVersion(ctx context.Context, dev *device.Device, versionID uuid.UUID) (*intelligence.Assignment, error)
	GetProfiler() *intelligence.Profiler
}

// Bridge integrates experiments into the heartbeat assignment flow.
type Bridge struct {
	service  *Service
	resolver VersionResolver
	logger   *slog.Logger
}

// NewBridge creates a new experiment bridge.
func NewBridge(service *Service, resolver VersionResolver, logger *slog.Logger) *Bridge {
	return &Bridge{service: service, resolver: resolver, logger: logger}
}

// PlanExperiments assigns the device to an arm of every running experiment
// in its project that targets it and returns the arm models it must run. The
// first experiment to claim a model wins; an arm whose version has no variant
// the device can run is skipped, leaving the model to deployments.
func (b *Bridge) PlanExperiments(ctx context.Context, dev *device.Device, currentlyLoaded []device.ModelStatus) (*device.ExperimentPlan, error) {
	plan := &device.ExperimentPlan{Keep: map[string]bool{}, Models: map[string]bool{}}
	if dev.ProjectID == nil {
		return plan, nil
	}
	exps, err := b.service.ListRunning(ctx)
	if err != nil {
		return nil, err
	}
	profile := b.resolver.GetProfiler().BuildProfile(dev, nil)

	for _, exp := range exps {
		if exp.ProjectID != *dev.ProjectID {
			continue
		}
		ts := intelligence.TargetSelector{Labels: exp.TargetSelector}
		if !ts.Matches(profile, dev.Labels) {
			continue
		}
		cohort, included, err := b.service.AssignDeviceCohort(ctx, dev.ID, exp.ID)
		if err != nil {
			if b.logger != nil {
				b.logger.Warn("cohort assignment failed", "experiment_id", exp.ID, "device_id", dev.ID, "error", err)
			}
			continue
		}
		if !included {
			continue
		}
		versionID := exp.ControlVersionID
		if cohort == "treatment" {
			versionID = exp.TreatmentVersionID
		}

		a, err := b.resolver.ResolveVersion(ctx, dev, versionID)
		if err != nil || a == nil {
			if err != nil && b.logger != nil {
				b.logger.Warn("resolving arm version failed", "experiment_id", exp.ID, "error", err)
			}
			continue
		}
		if plan.Models[a.ModelID] {
			continue // another experiment already controls this model
		}
		plan.Models[a.ModelID] = true
		plan.Keep[a.VariantID] = true
		plan.Experiments = append(plan.Experiments, device.ExperimentAssignment{
			ExperimentID:   exp.ID.String(),
			ArmName:        cohort,
			ArmID:          ArmID(exp.ID, cohort).String(),
			ModelVersionID: versionID.String(),
		})

		action := "LOAD"
		alreadyLoaded := false
		for _, m := range currentlyLoaded {
			if m.VariantID == a.VariantID {
				alreadyLoaded = true
				break
			}
			if m.ModelID == a.ModelID || m.ModelID == a.VersionID {
				action = "UPDATE"
			}
		}
		if alreadyLoaded {
			continue
		}
		plan.Assignments = append(plan.Assignments, device.ModelAssignment{
			ModelID:       a.ModelID,
			VersionID:     a.VersionID,
			ModelName:     a.ModelName,
			Version:       a.Version,
			VariantID:     a.VariantID,
			DownloadURL:   a.DownloadURL,
			BlobSizeBytes: uint64(a.BlobSizeBytes),
			BlobSHA256:    a.BlobSHA256,
			Action:        action,
		})
	}
	return plan, nil
}

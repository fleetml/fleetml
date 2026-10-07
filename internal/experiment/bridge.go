package experiment

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fleetml/fleetml/internal/device"
	"github.com/fleetml/fleetml/internal/intelligence"
)

// Bridge integrates experiments into the heartbeat assignment flow.
type Bridge struct {
	service      *Service
	orchestrator *intelligence.Orchestrator
	logger       *slog.Logger
}

// NewBridge creates a new experiment bridge.
func NewBridge(service *Service, orchestrator *intelligence.Orchestrator, logger *slog.Logger) *Bridge {
	return &Bridge{service: service, orchestrator: orchestrator, logger: logger}
}

// ComputeExperimentAssignments returns model assignments based on running experiments.
func (b *Bridge) ComputeExperimentAssignments(
	ctx context.Context,
	dev *device.Device,
	currentlyLoaded []device.ModelStatus,
) ([]device.ModelAssignment, error) {
	exps, err := b.service.ListRunning(ctx)
	if err != nil {
		return nil, err
	}

	profile := b.orchestrator.GetProfiler().BuildProfile(dev, nil)
	var assignments []device.ModelAssignment

	for _, exp := range exps {
		// Check targeting
		ts := intelligence.TargetSelector{Labels: exp.TargetSelector}
		if !ts.Matches(profile, dev.Labels) {
			continue
		}

		// Assign cohort
		cohort, included, err := b.service.AssignDeviceCohort(ctx, dev.ID, exp.ID)
		if err != nil || !included {
			continue
		}

		// Determine which version to use
		var versionID uuid.UUID
		if cohort == "treatment" {
			versionID = exp.TreatmentVersionID
		} else {
			versionID = exp.ControlVersionID
		}

		// Load variants and select best for device
		candidates, err := b.orchestrator.LoadVariantsForVersion(ctx, versionID)
		if err != nil || len(candidates) == 0 {
			continue
		}

		result := b.orchestrator.GetSelector().Select(profile, candidates)
		if result == nil {
			continue
		}

		// Generate download URL
		downloadURL := ""
		for _, c := range candidates {
			if c.VariantID == result.VariantID {
				downloadURL, _ = generateURL(b.orchestrator, ctx, c.BlobKey)
				break
			}
		}

		// Check if already loaded
		action := "LOAD"
		alreadyLoaded := false
		for _, loaded := range currentlyLoaded {
			if loaded.VariantID == result.VariantID.String() {
				alreadyLoaded = true
				break
			}
			if loaded.ModelID == versionID.String() {
				action = "UPDATE"
			}
		}
		if alreadyLoaded {
			continue
		}

		assignments = append(assignments, device.ModelAssignment{
			ModelID:       versionID.String(),
			ModelName:     exp.Name,
			Version:       cohort,
			VariantID:     result.VariantID.String(),
			DownloadURL:   downloadURL,
			BlobSizeBytes: 0,
			BlobSHA256:    "",
			Action:        action,
		})
	}
	return assignments, nil
}

func generateURL(orch *intelligence.Orchestrator, ctx context.Context, blobKey string) (string, error) {
	// Use the orchestrator's S3 client via a helper
	return "", nil // URL generation handled at orchestrator level
}

// ExperimentVariantAdapter loads variants from the database for experiment bridge.
type ExperimentVariantAdapter struct {
	pool *pgxpool.Pool
}

// NewExperimentVariantAdapter creates a new adapter.
func NewExperimentVariantAdapter(pool *pgxpool.Pool) *ExperimentVariantAdapter {
	return &ExperimentVariantAdapter{pool: pool}
}

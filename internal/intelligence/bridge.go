package intelligence

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/device"
)

// DeploymentLister loads a project's active deployments.
type DeploymentLister interface {
	ListActiveDeployments(ctx context.Context, projectID uuid.UUID) ([]DeploymentData, error)
}

// Bridge adapts the Orchestrator to the device.AssignmentComputer interface.
type Bridge struct {
	orchestrator *Orchestrator
	deployments  DeploymentLister
}

// NewBridge creates a new Bridge.
func NewBridge(orchestrator *Orchestrator, deployments DeploymentLister) *Bridge {
	return &Bridge{orchestrator: orchestrator, deployments: deployments}
}

// ComputeAssignments implements device.AssignmentComputer. A device only ever
// sees deployments of the project it enrolled into; a device without a
// project gets none.
func (b *Bridge) ComputeAssignments(ctx context.Context, dev *device.Device, currentlyLoaded []device.ModelStatus) ([]device.ModelAssignment, error) {
	if dev.ProjectID == nil {
		return nil, nil
	}
	deployments, err := b.deployments.ListActiveDeployments(ctx, *dev.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("listing active deployments: %w", err)
	}

	loaded := make([]LoadedModelInfo, 0, len(currentlyLoaded))
	for _, m := range currentlyLoaded {
		loaded = append(loaded, LoadedModelInfo{
			ModelID:   m.ModelID,
			VariantID: m.VariantID,
			Version:   m.Version,
		})
	}

	assignments, err := b.orchestrator.ComputeAssignments(ctx, dev, nil, deployments, loaded)
	if err != nil {
		return nil, err
	}

	// Convert to device.ModelAssignment
	result := make([]device.ModelAssignment, 0, len(assignments))
	for _, a := range assignments {
		result = append(result, device.ModelAssignment{
			ModelID:       a.ModelID,
			VersionID:     a.VersionID,
			ModelName:     a.ModelName,
			Version:       a.Version,
			VariantID:     a.VariantID,
			DownloadURL:   a.DownloadURL,
			BlobSizeBytes: uint64(a.BlobSizeBytes),
			BlobSHA256:    a.BlobSHA256,
			Action:        a.Action,
		})
	}
	return result, nil
}

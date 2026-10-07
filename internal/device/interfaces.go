package device

import (
	"context"

	"github.com/google/uuid"
)

// DeploymentQuerier queries active deployments.
type DeploymentQuerier interface {
	ListActiveForDevice(ctx context.Context, deviceLabels map[string]string) ([]DeploymentInfo, error)
}

// DeploymentInfo holds deployment data needed for model assignments.
type DeploymentInfo struct {
	ID             uuid.UUID
	ModelVersionID uuid.UUID
	Strategy       string
	CanaryPct      int
	Status         string
}

// VariantQuerier queries model variants for a version.
type VariantQuerier interface {
	ListByVersion(ctx context.Context, versionID uuid.UUID) ([]VariantInfo, error)
}

// VariantInfo holds variant data needed for model assignments.
type VariantInfo struct {
	ID            uuid.UUID
	VersionID     uuid.UUID
	Quantization  string
	TargetRuntime string
	MinRAMMB      int
	MinNPUTflops  *float64
	BlobKey       string
	BlobSizeBytes int64
	BlobSHA256    string
}

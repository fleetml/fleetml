package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/device"
	"github.com/fleetml/fleetml/internal/rollout"
)

const downloadURLExpiry = 1 * time.Hour

// VariantRepository loads variants for a model version.
type VariantRepository interface {
	GetByVersionID(ctx context.Context, versionID uuid.UUID) ([]VariantData, error)
}

// StorageClient generates presigned download URLs.
type StorageClient interface {
	GeneratePresignedDownload(ctx context.Context, key string, expiry time.Duration) (string, error)
}

// VariantData holds variant info loaded from the database.
type VariantData struct {
	ID            uuid.UUID
	VersionID     uuid.UUID
	ModelID       uuid.UUID
	ModelName     string
	VersionString string
	Quantization  string
	Runtime       string
	BlobKey       string
	BlobSize      int64
	BlobSHA256    string
	MetadataJSON  map[string]interface{}
}

// DeploymentData holds deployment info for assignment computation.
type DeploymentData struct {
	ID             uuid.UUID
	ModelID        uuid.UUID
	ModelVersionID uuid.UUID
	ModelName      string
	VersionString  string
	TargetSelector TargetSelector
	Strategy       string
	Status         string // ROLLING_OUT or COMPLETE
	CanaryPercent  int
	// RolloutThreshold is the ring position below which a rolling
	// deployment's devices are exposed.
	RolloutThreshold int64
	Urgent           bool
}

// exposes reports whether the deployment currently delivers to this device.
// A COMPLETE deployment covers every targeted device; a canary covers its
// current stage's slice of the hash ring.
func (d DeploymentData) exposes(deviceID uuid.UUID) bool {
	if d.Status == "COMPLETE" {
		return true
	}
	if d.Strategy == "CANARY" && d.CanaryPercent > 0 && d.CanaryPercent < 100 {
		return rollout.InCanary(deviceID, d.ID, d.CanaryPercent)
	}
	if d.Strategy == "ROLLING" {
		return rollout.Exposed(deviceID, d.ID, d.RolloutThreshold)
	}
	return true
}

// LoadedModelInfo represents a model currently loaded on a device.
type LoadedModelInfo struct {
	ModelID   string
	VariantID string
	Version   string
}

// Assignment tells a device what to do with a model.
type Assignment struct {
	ModelID       string // the model, stable across versions
	VersionID     string // the model version being delivered
	ModelName     string
	Version       string
	VariantID     string
	DownloadURL   string
	BlobSizeBytes int64
	BlobSHA256    string
	Action        string // "LOAD", "UPDATE", "UNLOAD"
	Reason        string
	Urgent        bool // download on any network
}

// Orchestrator coordinates profiling, targeting, and selection.
type Orchestrator struct {
	profiler    *Profiler
	selector    *Selector
	variantRepo VariantRepository
	s3Client    StorageClient
	logger      *slog.Logger
}

// NewOrchestrator creates a new Orchestrator.
func NewOrchestrator(
	profiler *Profiler,
	selector *Selector,
	variantRepo VariantRepository,
	s3Client StorageClient,
	logger *slog.Logger,
) *Orchestrator {
	return &Orchestrator{
		profiler:    profiler,
		selector:    selector,
		variantRepo: variantRepo,
		s3Client:    s3Client,
		logger:      logger,
	}
}

// GetProfiler returns the profiler for external use (e.g., REST profile endpoint).
func (o *Orchestrator) GetProfiler() *Profiler {
	return o.profiler
}

// GetSelector returns the selector for external use (e.g., REST preview endpoint).
func (o *Orchestrator) GetSelector() *Selector {
	return o.selector
}

// ComputeAssignments determines which models a device should have.
//
// activeDeployments must be ordered newest first. For each model the device
// gets the newest deployment that targets it, exposes it and has a variant it
// can run; older deployments of the same model are superseded. So devices
// outside a canary keep the previously completed version, and rolling back or
// cancelling a deployment hands its devices back to the one before it.
func (o *Orchestrator) ComputeAssignments(
	ctx context.Context,
	dev *device.Device,
	health *HealthSnapshot,
	activeDeployments []DeploymentData,
	currentlyLoaded []LoadedModelInfo,
) ([]Assignment, error) {
	profile := o.profiler.BuildProfile(dev, health)

	loadedVariants := make(map[string]LoadedModelInfo)
	for _, m := range currentlyLoaded {
		loadedVariants[m.VariantID] = m
	}

	activeVariantIDs := make(map[string]bool)
	servedModels := make(map[uuid.UUID]bool)
	var desired []Assignment

	for _, dep := range activeDeployments {
		if dep.ModelID != uuid.Nil && servedModels[dep.ModelID] {
			continue // superseded by a newer deployment of the same model
		}
		if !dep.TargetSelector.Matches(profile, dev.Labels) {
			if o.logger != nil {
				o.logger.Debug("device does not match deployment targeting",
					"device_id", dev.ID, "deployment_id", dep.ID, "device_class", profile.DeviceClass)
			}
			continue
		}

		if !dep.exposes(dev.ID) {
			continue
		}

		variants, err := o.variantRepo.GetByVersionID(ctx, dep.ModelVersionID)
		if err != nil {
			if o.logger != nil {
				o.logger.Warn("failed to load variants", "deployment_id", dep.ID, "error", err)
			}
			continue
		}

		candidates := make([]CandidateVariant, 0, len(variants))
		for _, v := range variants {
			candidates = append(candidates, CandidateVariant{
				VariantID:    v.ID,
				VersionID:    v.VersionID,
				Quantization: v.Quantization,
				Runtime:      v.Runtime,
				BlobKey:      v.BlobKey,
				BlobSize:     v.BlobSize,
				BlobSHA256:   v.BlobSHA256,
				Requirements: ParseRequirements(v.MetadataJSON, v.Quantization, v.Runtime, v.BlobSize),
			})
		}

		result := o.selector.Select(profile, candidates)
		if result == nil {
			if o.logger != nil {
				o.logger.Debug("no compatible variant", "deployment_id", dep.ID, "device_class", profile.DeviceClass,
					"candidates", len(candidates))
			}
			continue
		}

		activeVariantIDs[result.VariantID.String()] = true
		if dep.ModelID != uuid.Nil {
			servedModels[dep.ModelID] = true
		}

		if _, loaded := loadedVariants[result.VariantID.String()]; loaded {
			continue
		}

		action := "LOAD"
		for _, loaded := range currentlyLoaded {
			// Older SDKs echo the version ID as model_id; accept both.
			if loaded.ModelID == dep.ModelID.String() || loaded.ModelID == dep.ModelVersionID.String() {
				action = "UPDATE"
				break
			}
		}

		// Generate download URL
		downloadURL := ""
		for _, v := range variants {
			if v.ID == result.VariantID {
				downloadURL, _ = o.s3Client.GeneratePresignedDownload(ctx, v.BlobKey, downloadURLExpiry)
				break
			}
		}

		desired = append(desired, Assignment{
			ModelID:       dep.ModelID.String(),
			VersionID:     dep.ModelVersionID.String(),
			ModelName:     dep.ModelName,
			Version:       dep.VersionString,
			VariantID:     result.VariantID.String(),
			DownloadURL:   downloadURL,
			BlobSizeBytes: findBlobSize(variants, result.VariantID),
			BlobSHA256:    findBlobSHA(variants, result.VariantID),
			Action:        action,
			Reason:        result.Reason,
			Urgent:        dep.Urgent,
		})
	}

	// Unload models no longer deployed
	for variantID, loaded := range loadedVariants {
		if !activeVariantIDs[variantID] {
			desired = append(desired, Assignment{
				ModelID:   loaded.ModelID,
				VariantID: variantID,
				Action:    "UNLOAD",
				Reason:    "deployment no longer active",
			})
		}
	}

	return desired, nil
}

// ResolveVersion picks the variant of one model version that a device should
// run and returns a LOAD assignment for it with a presigned URL and sha256,
// or nil if no READY variant is compatible with the device.
func (o *Orchestrator) ResolveVersion(ctx context.Context, dev *device.Device, versionID uuid.UUID) (*Assignment, error) {
	variants, err := o.variantRepo.GetByVersionID(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("loading variants: %w", err)
	}
	candidates := make([]CandidateVariant, 0, len(variants))
	for _, v := range variants {
		candidates = append(candidates, CandidateVariant{
			VariantID: v.ID, VersionID: v.VersionID, Quantization: v.Quantization, Runtime: v.Runtime,
			BlobKey: v.BlobKey, BlobSize: v.BlobSize, BlobSHA256: v.BlobSHA256,
			Requirements: ParseRequirements(v.MetadataJSON, v.Quantization, v.Runtime, v.BlobSize),
		})
	}
	result := o.selector.Select(o.profiler.BuildProfile(dev, nil), candidates)
	if result == nil {
		return nil, nil
	}
	for _, v := range variants {
		if v.ID != result.VariantID {
			continue
		}
		url, err := o.s3Client.GeneratePresignedDownload(ctx, v.BlobKey, downloadURLExpiry)
		if err != nil {
			return nil, fmt.Errorf("presigning download: %w", err)
		}
		return &Assignment{
			ModelID: v.ModelID.String(), VersionID: v.VersionID.String(), ModelName: v.ModelName,
			Version: v.VersionString, VariantID: v.ID.String(), DownloadURL: url,
			BlobSizeBytes: v.BlobSize, BlobSHA256: v.BlobSHA256, Action: "LOAD", Reason: result.Reason,
		}, nil
	}
	return nil, nil
}

// LoadVariantsForVersion loads and converts variants for the preview endpoint.
func (o *Orchestrator) LoadVariantsForVersion(ctx context.Context, versionID uuid.UUID) ([]CandidateVariant, error) {
	variants, err := o.variantRepo.GetByVersionID(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("loading variants: %w", err)
	}
	candidates := make([]CandidateVariant, 0, len(variants))
	for _, v := range variants {
		candidates = append(candidates, CandidateVariant{
			VariantID:    v.ID,
			VersionID:    v.VersionID,
			Quantization: v.Quantization,
			Runtime:      v.Runtime,
			BlobKey:      v.BlobKey,
			BlobSize:     v.BlobSize,
			BlobSHA256:   v.BlobSHA256,
			Requirements: ParseRequirements(v.MetadataJSON, v.Quantization, v.Runtime, v.BlobSize),
		})
	}
	return candidates, nil
}

func findBlobSize(variants []VariantData, id uuid.UUID) int64 {
	for _, v := range variants {
		if v.ID == id {
			return v.BlobSize
		}
	}
	return 0
}

func findBlobSHA(variants []VariantData, id uuid.UUID) string {
	for _, v := range variants {
		if v.ID == id {
			return v.BlobSHA256
		}
	}
	return ""
}

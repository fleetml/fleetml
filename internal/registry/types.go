package registry

import (
	"time"

	"github.com/google/uuid"
)

// Project represents a top-level organizational unit for models.
type Project struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	OrgID       *uuid.UUID `json:"org_id"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// RegistryModel represents an ML model registered under a project.
type RegistryModel struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Framework   string    `json:"framework"`
	TaskType    string    `json:"task_type"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by"`
}

// RegistryVersion represents a semver-tagged version of a model.
type RegistryVersion struct {
	ID           uuid.UUID              `json:"id"`
	ModelID      uuid.UUID              `json:"model_id"`
	Version      string                 `json:"version"`
	Status       string                 `json:"status"`
	MetadataJSON map[string]interface{} `json:"metadata"`
	ReleaseNotes string                 `json:"release_notes"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	PromotedAt   *time.Time             `json:"promoted_at"`
	PromotedBy   *string                `json:"promoted_by"`
}

// Variant statuses. Only READY variants are assigned to devices.
const (
	VariantUploading = "UPLOADING"
	VariantReady     = "READY"
	VariantFailed    = "FAILED"
)

// RegistryVariant represents a specific quantization/runtime variant of a model version.
type RegistryVariant struct {
	ID                 uuid.UUID `json:"id"`
	VersionID          uuid.UUID `json:"version_id"`
	Quantization       string    `json:"quantization"`
	TargetRuntime      string    `json:"target_runtime"`
	MinRAMMB           int       `json:"min_ram_mb"`
	MinNPUTflops       *float64  `json:"min_npu_tflops"`
	BlobKey            string    `json:"blob_key"`
	BlobSizeBytes      int64     `json:"blob_size_bytes"`
	BlobSHA256         string    `json:"blob_sha256"`
	BenchmarkLatencyMs *float64  `json:"benchmark_latency_ms"`
	BenchmarkAccuracy  *float64  `json:"benchmark_accuracy"`
	Status             string    `json:"status"` // UPLOADING, READY, FAILED
	CreatedAt          time.Time `json:"created_at"`
}

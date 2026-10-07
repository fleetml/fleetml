package registry

import (
	"time"

	"github.com/google/uuid"
)

// Project represents a top-level organizational unit for models.
type Project struct {
	ID          uuid.UUID
	Name        string
	Slug        string
	OrgID       *uuid.UUID
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RegistryModel represents an ML model registered under a project.
type RegistryModel struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	Name        string
	Description string
	Framework   string
	TaskType    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatedBy   string
}

// RegistryVersion represents a semver-tagged version of a model.
type RegistryVersion struct {
	ID           uuid.UUID
	ModelID      uuid.UUID
	Version      string
	Status       string
	MetadataJSON map[string]interface{}
	ReleaseNotes string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	PromotedAt   *time.Time
	PromotedBy   *string
}

// RegistryVariant represents a specific quantization/runtime variant of a model version.
type RegistryVariant struct {
	ID                 uuid.UUID
	VersionID          uuid.UUID
	Quantization       string
	TargetRuntime      string
	MinRAMMB           int
	MinNPUTflops       *float64
	BlobKey            string
	BlobSizeBytes      int64
	BlobSHA256         string
	BenchmarkLatencyMs *float64
	BenchmarkAccuracy  *float64
	CreatedAt          time.Time
}

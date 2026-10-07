package registry

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("already exists")
	ErrInvalidInput = errors.New("invalid input")
)

type Model struct {
	ID          string
	ProjectID   string
	Name        string
	Description string
	Framework   string
	TaskType    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CreatedBy   string
}

type ModelVersion struct {
	ID           string
	ModelID      string
	Version      string
	Status       string
	MetadataJSON map[string]any
	ReleaseNotes string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	PromotedAt   *time.Time
	PromotedBy   *string
}

type ModelVariant struct {
	ID                 string
	VersionID          string
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

type ModelStore interface {
	CreateModel(ctx context.Context, m *Model) error
	GetModel(ctx context.Context, id string) (*Model, error)
	ListModels(ctx context.Context, projectID string) ([]*Model, error)
	DeleteModel(ctx context.Context, id string) error
}

type VersionStore interface {
	CreateVersion(ctx context.Context, v *ModelVersion) error
	GetVersion(ctx context.Context, id string) (*ModelVersion, error)
	ListVersions(ctx context.Context, modelID string) ([]*ModelVersion, error)
	UpdateVersionStatus(ctx context.Context, id string, status string) error
}

type VariantStore interface {
	CreateVariant(ctx context.Context, v *ModelVariant) error
	GetVariant(ctx context.Context, id string) (*ModelVariant, error)
	ListVariants(ctx context.Context, versionID string) ([]*ModelVariant, error)
}

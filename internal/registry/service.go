package registry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidTransition = errors.New("invalid status transition")
	// ErrIntegrity means an uploaded blob does not match its declared sha256.
	ErrIntegrity = errors.New("integrity check failed")

	validFrameworks    = map[string]bool{"TFLITE": true, "ONNX": true, "COREML": true, "GGUF": true, "CUSTOM": true}
	validTaskTypes     = map[string]bool{"CLASSIFICATION": true, "DETECTION": true, "SEGMENTATION": true, "LLM": true, "STT": true, "TTS": true, "VLM": true, "RECOMMENDATION": true, "ANOMALY": true, "CUSTOM": true}
	validQuantizations = map[string]bool{"FP32": true, "FP16": true, "INT8": true, "INT4": true, "DYNAMIC": true}
	validRuntimes      = map[string]bool{"TFLITE": true, "ONNX": true, "COREML": true, "GGUF": true, "CUSTOM": true}

	// validTransitions maps current status to allowed next statuses.
	validTransitions = map[string]map[string]bool{
		"DRAFT":    {"STAGED": true, "ARCHIVED": true},
		"STAGED":   {"ACTIVE": true},
		"ACTIVE":   {"ARCHIVED": true},
		"ARCHIVED": {},
	}
)

const (
	UploadURLExpiry   = 15 * time.Minute
	DownloadURLExpiry = 1 * time.Hour
)

// StorageClient defines the S3 operations the service needs.
type StorageClient interface {
	GeneratePresignedUpload(ctx context.Context, key string, expiry time.Duration) (string, error)
	GeneratePresignedDownload(ctx context.Context, key string, expiry time.Duration) (string, error)
	ObjectExists(ctx context.Context, key string) (bool, error)
	DeleteObject(ctx context.Context, key string) error
	GetObjectInfo(ctx context.Context, key string) (int64, error)
	OpenObject(ctx context.Context, key string) (io.ReadCloser, error)
}

// AuditLogger defines the audit logging interface the service needs.
type AuditLogger interface {
	Log(ctx context.Context, actor, action, resourceType, resourceID string, details map[string]any) error
}

// Service orchestrates business logic for the model registry.
type Service struct {
	projects ProjectRepository
	models   ModelRepository
	versions VersionRepository
	variants VariantRepository
	storage  StorageClient
	audit    AuditLogger
	logger   *slog.Logger
	signer   VariantSigner
	onReady  func(variantID string)
}

// VariantSigner signs a verified variant (implemented by *signing.Keyring).
type VariantSigner interface {
	SignVariant(ctx context.Context, variantID string) error
}

// SetSigner makes FinalizeVariant sign every variant before it becomes READY.
func (s *Service) SetSigner(signer VariantSigner) { s.signer = signer }

// SetReadyHook is called with every variant that becomes READY (to build its delta).
func (s *Service) SetReadyHook(f func(variantID string)) { s.onReady = f }

// NewService creates a new registry service.
func NewService(
	projects ProjectRepository,
	models ModelRepository,
	versions VersionRepository,
	variants VariantRepository,
	storage StorageClient,
	audit AuditLogger,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		projects: projects,
		models:   models,
		versions: versions,
		variants: variants,
		storage:  storage,
		audit:    audit,
		logger:   logger,
	}
}

// --- Slug generation ---

func GenerateSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	// Replace non-alphanumeric with hyphens
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	slug := b.String()
	// Collapse multiple hyphens
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	// Ensure it starts with a letter
	if slug != "" && (slug[0] < 'a' || slug[0] > 'z') {
		slug = "p-" + slug
	}
	if len(slug) > 63 {
		slug = slug[:63]
	}
	slug = strings.TrimRight(slug, "-")
	if slug == "" {
		slug = "project"
	}
	return slug
}

// --- Projects ---

func (s *Service) CreateProject(ctx context.Context, name, description string) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return nil, fmt.Errorf("%w: project name must be 1-255 characters", ErrInvalidInput)
	}

	slug := GenerateSlug(name)

	p := &Project{
		Name:        name,
		Slug:        slug,
		Description: description,
	}
	if err := s.projects.Create(ctx, p); err != nil {
		return nil, err
	}
	s.audit.Log(ctx, "system", "create", "project", p.ID.String(), map[string]any{"name": name, "slug": slug}) //nolint:errcheck // audit logging is best-effort
	return p, nil
}

func (s *Service) GetProject(ctx context.Context, id uuid.UUID) (*Project, error) {
	return s.projects.GetByID(ctx, id)
}

func (s *Service) GetProjectBySlug(ctx context.Context, slug string) (*Project, error) {
	return s.projects.GetBySlug(ctx, slug)
}

func (s *Service) ListProjects(ctx context.Context, limit, offset int) ([]Project, int, error) {
	limit, offset = clampPagination(limit, offset)
	return s.projects.List(ctx, limit, offset)
}

func (s *Service) UpdateProject(ctx context.Context, id uuid.UUID, name, description string) (*Project, error) {
	p, err := s.projects.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if name != "" {
		name = strings.TrimSpace(name)
		if len(name) > 255 {
			return nil, fmt.Errorf("%w: project name must be 1-255 characters", ErrInvalidInput)
		}
		p.Name = name
		p.Slug = GenerateSlug(name)
	}
	p.Description = description

	if err := s.projects.Update(ctx, p); err != nil {
		return nil, err
	}
	s.audit.Log(ctx, "system", "update", "project", p.ID.String(), nil) //nolint:errcheck // audit logging is best-effort
	return p, nil
}

func (s *Service) DeleteProject(ctx context.Context, id uuid.UUID) error {
	if err := s.projects.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Log(ctx, "system", "delete", "project", id.String(), nil) //nolint:errcheck // audit logging is best-effort
	return nil
}

// --- Models ---

func (s *Service) CreateModel(ctx context.Context, projectID uuid.UUID, name, description, framework, taskType, createdBy string) (*RegistryModel, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return nil, fmt.Errorf("%w: model name must be 1-255 characters", ErrInvalidInput)
	}
	framework = strings.ToUpper(strings.TrimSpace(framework))
	if !validFrameworks[framework] {
		return nil, fmt.Errorf("%w: framework must be one of TFLITE, ONNX, COREML, GGUF, CUSTOM", ErrInvalidInput)
	}
	taskType = strings.ToUpper(strings.TrimSpace(taskType))
	if !validTaskTypes[taskType] {
		return nil, fmt.Errorf("%w: task_type must be one of CLASSIFICATION, DETECTION, SEGMENTATION, LLM, STT, TTS, VLM, RECOMMENDATION, ANOMALY, CUSTOM", ErrInvalidInput)
	}
	if createdBy == "" {
		createdBy = "system"
	}

	// Verify project exists
	if _, err := s.projects.GetByID(ctx, projectID); err != nil {
		return nil, err
	}

	m := &RegistryModel{
		ProjectID:   projectID,
		Name:        name,
		Description: description,
		Framework:   framework,
		TaskType:    taskType,
		CreatedBy:   createdBy,
	}
	if err := s.models.Create(ctx, m); err != nil {
		return nil, err
	}
	s.audit.Log(ctx, createdBy, "create", "model", m.ID.String(), map[string]any{"name": name}) //nolint:errcheck // audit logging is best-effort
	return m, nil
}

func (s *Service) GetModel(ctx context.Context, id uuid.UUID) (*RegistryModel, error) {
	return s.models.GetByID(ctx, id)
}

func (s *Service) ListModels(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]RegistryModel, int, error) {
	limit, offset = clampPagination(limit, offset)
	return s.models.ListByProject(ctx, projectID, limit, offset)
}

func (s *Service) UpdateModel(ctx context.Context, id uuid.UUID, name, description, framework, taskType string) (*RegistryModel, error) {
	m, err := s.models.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if name != "" {
		name = strings.TrimSpace(name)
		if len(name) > 255 {
			return nil, fmt.Errorf("%w: model name must be 1-255 characters", ErrInvalidInput)
		}
		m.Name = name
	}
	m.Description = description
	if framework != "" {
		framework = strings.ToUpper(strings.TrimSpace(framework))
		if !validFrameworks[framework] {
			return nil, fmt.Errorf("%w: framework must be one of TFLITE, ONNX, COREML, GGUF, CUSTOM", ErrInvalidInput)
		}
		m.Framework = framework
	}
	if taskType != "" {
		taskType = strings.ToUpper(strings.TrimSpace(taskType))
		if !validTaskTypes[taskType] {
			return nil, fmt.Errorf("%w: task_type must be one of CLASSIFICATION, DETECTION, SEGMENTATION, LLM, STT, TTS, VLM, RECOMMENDATION, ANOMALY, CUSTOM", ErrInvalidInput)
		}
		m.TaskType = taskType
	}

	if err := s.models.Update(ctx, m); err != nil {
		return nil, err
	}
	s.audit.Log(ctx, "system", "update", "model", m.ID.String(), nil) //nolint:errcheck // audit logging is best-effort
	return m, nil
}

func (s *Service) DeleteModel(ctx context.Context, id uuid.UUID) error {
	if err := s.models.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Log(ctx, "system", "delete", "model", id.String(), nil) //nolint:errcheck // audit logging is best-effort
	return nil
}

// --- Versions ---

func (s *Service) CreateVersion(ctx context.Context, modelID uuid.UUID, version, releaseNotes string, metadata map[string]interface{}) (*RegistryVersion, error) {
	version = strings.TrimSpace(version)
	if err := ValidateSemver(version); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, err.Error())
	}

	// Verify model exists
	if _, err := s.models.GetByID(ctx, modelID); err != nil {
		return nil, err
	}

	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	v := &RegistryVersion{
		ModelID:      modelID,
		Version:      version,
		Status:       "DRAFT",
		MetadataJSON: metadata,
		ReleaseNotes: releaseNotes,
	}
	if err := s.versions.Create(ctx, v); err != nil {
		return nil, err
	}
	s.audit.Log(ctx, "system", "create", "model_version", v.ID.String(), map[string]any{"version": version}) //nolint:errcheck // audit logging is best-effort
	return v, nil
}

func (s *Service) GetVersion(ctx context.Context, id uuid.UUID) (*RegistryVersion, error) {
	return s.versions.GetByID(ctx, id)
}

func (s *Service) ListVersions(ctx context.Context, modelID uuid.UUID, limit, offset int) ([]RegistryVersion, int, error) {
	limit, offset = clampPagination(limit, offset)
	return s.versions.ListByModel(ctx, modelID, limit, offset)
}

func (s *Service) UpdateVersionStatus(ctx context.Context, id uuid.UUID, newStatus string, promotedBy *string) (*RegistryVersion, error) {
	newStatus = strings.ToUpper(strings.TrimSpace(newStatus))

	v, err := s.versions.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	allowed, ok := validTransitions[v.Status]
	if !ok || !allowed[newStatus] {
		return nil, fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, v.Status, newStatus)
	}

	if err := s.versions.UpdateStatus(ctx, id, newStatus, promotedBy); err != nil {
		return nil, err
	}

	s.audit.Log(ctx, "system", "update_status", "model_version", id.String(), //nolint:errcheck // audit logging is best-effort
		map[string]any{"from": v.Status, "to": newStatus})

	return s.versions.GetByID(ctx, id)
}

func (s *Service) DeleteVersion(ctx context.Context, id uuid.UUID) error {
	if err := s.versions.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Log(ctx, "system", "delete", "model_version", id.String(), nil) //nolint:errcheck // audit logging is best-effort
	return nil
}

// --- Variants ---

// CreateVariantInput holds the metadata for creating a variant.
type CreateVariantInput struct {
	VersionID          uuid.UUID
	Quantization       string
	TargetRuntime      string
	MinRAMMB           int
	MinNPUTflops       *float64
	BlobSizeBytes      int64
	BlobSHA256         string
	BenchmarkLatencyMs *float64
	BenchmarkAccuracy  *float64
}

// CreateVariantOutput holds the result of variant creation.
type CreateVariantOutput struct {
	Variant   *RegistryVariant
	UploadURL string
}

func (s *Service) CreateVariant(ctx context.Context, input CreateVariantInput) (*CreateVariantOutput, error) {
	input.Quantization = strings.ToUpper(strings.TrimSpace(input.Quantization))
	if !validQuantizations[input.Quantization] {
		return nil, fmt.Errorf("%w: quantization must be one of FP32, FP16, INT8, INT4, DYNAMIC", ErrInvalidInput)
	}
	input.TargetRuntime = strings.ToUpper(strings.TrimSpace(input.TargetRuntime))
	if !validRuntimes[input.TargetRuntime] {
		return nil, fmt.Errorf("%w: target_runtime must be one of TFLITE, ONNX, COREML, GGUF, CUSTOM", ErrInvalidInput)
	}
	if input.BlobSizeBytes <= 0 {
		return nil, fmt.Errorf("%w: blob_size_bytes must be positive", ErrInvalidInput)
	}
	if input.BlobSHA256 == "" {
		return nil, fmt.Errorf("%w: blob_sha256 is required", ErrInvalidInput)
	}

	// Verify version exists and look up model + project for S3 key
	ver, err := s.versions.GetByID(ctx, input.VersionID)
	if err != nil {
		return nil, err
	}
	model, err := s.models.GetByID(ctx, ver.ModelID)
	if err != nil {
		return nil, err
	}
	proj, err := s.projects.GetByID(ctx, model.ProjectID)
	if err != nil {
		return nil, err
	}

	blobKey := fmt.Sprintf("models/%s/%s/%s/%s_%s.bin",
		proj.Slug, model.Name, ver.Version, input.Quantization, input.TargetRuntime)

	v := &RegistryVariant{
		VersionID:          input.VersionID,
		Quantization:       input.Quantization,
		TargetRuntime:      input.TargetRuntime,
		MinRAMMB:           input.MinRAMMB,
		MinNPUTflops:       input.MinNPUTflops,
		BlobKey:            blobKey,
		BlobSizeBytes:      input.BlobSizeBytes,
		BlobSHA256:         input.BlobSHA256,
		BenchmarkLatencyMs: input.BenchmarkLatencyMs,
		BenchmarkAccuracy:  input.BenchmarkAccuracy,
		Status:             VariantUploading,
	}

	if err := s.variants.Create(ctx, v); err != nil {
		if !errors.Is(err, ErrConflict) {
			return nil, err
		}
		if err := s.replaceUnfinalizedVariant(ctx, v); err != nil {
			return nil, err
		}
	}

	uploadURL, err := s.storage.GeneratePresignedUpload(ctx, blobKey, UploadURLExpiry)
	if err != nil {
		return nil, fmt.Errorf("generating upload URL: %w", err)
	}

	s.audit.Log(ctx, "system", "create", "model_variant", v.ID.String(), //nolint:errcheck // audit logging is best-effort
		map[string]any{"quantization": input.Quantization, "runtime": input.TargetRuntime})

	return &CreateVariantOutput{Variant: v, UploadURL: uploadURL}, nil
}

// replaceUnfinalizedVariant creates v in place of the version's variant with
// the same quantization and runtime when that one was never verified (its
// upload failed or never happened, or finalize refused it), so a push can be
// retried. A READY variant is never replaced.
func (s *Service) replaceUnfinalizedVariant(ctx context.Context, v *RegistryVariant) error {
	existing, err := s.variants.ListByVersion(ctx, v.VersionID)
	if err != nil {
		return err
	}
	for _, old := range existing {
		if old.Quantization != v.Quantization || old.TargetRuntime != v.TargetRuntime {
			continue
		}
		if old.Status == VariantReady {
			return fmt.Errorf("%w: variant %s/%s of this version is already uploaded and verified; push a new version",
				ErrConflict, v.Quantization, v.TargetRuntime)
		}
		if err := s.variants.Delete(ctx, old.ID); err != nil {
			return fmt.Errorf("replacing unfinalized variant: %w", err)
		}
		if err := s.variants.Create(ctx, v); err != nil {
			return err
		}
		s.logger.Info("replaced an unfinalized variant", "old_variant_id", old.ID, "variant_id", v.ID, "old_status", old.Status)
		s.audit.Log(ctx, "system", "replace", "model_variant", v.ID.String(), //nolint:errcheck // audit logging is best-effort
			map[string]any{"replaced_variant_id": old.ID.String(), "replaced_status": old.Status})
		return nil
	}
	return fmt.Errorf("%w: variant %s/%s for version", ErrConflict, v.Quantization, v.TargetRuntime)
}

func (s *Service) GetVariant(ctx context.Context, id uuid.UUID) (*RegistryVariant, error) {
	return s.variants.GetByID(ctx, id)
}

func (s *Service) ListVariants(ctx context.Context, versionID uuid.UUID) ([]RegistryVariant, error) {
	return s.variants.ListByVersion(ctx, versionID)
}

func (s *Service) GetVariantDownloadURL(ctx context.Context, id uuid.UUID) (string, error) {
	v, err := s.variants.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	return s.storage.GeneratePresignedDownload(ctx, v.BlobKey, DownloadURLExpiry)
}

// FinalizeVariant verifies an uploaded blob and marks the variant READY.
// The blob is streamed from storage and its sha256 compared with the declared
// one; on mismatch the blob is deleted, the variant is marked FAILED and
// ErrIntegrity is returned. Only READY variants are assigned to devices.
func (s *Service) FinalizeVariant(ctx context.Context, id uuid.UUID) (*RegistryVariant, error) {
	v, err := s.variants.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	exists, err := s.storage.ObjectExists(ctx, v.BlobKey)
	if err != nil {
		return nil, fmt.Errorf("checking blob existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: blob not yet uploaded to S3", ErrInvalidInput)
	}

	size, err := s.storage.GetObjectInfo(ctx, v.BlobKey)
	if err != nil {
		return nil, fmt.Errorf("getting blob info: %w", err)
	}
	if size != v.BlobSizeBytes {
		return nil, s.failVariant(ctx, v, fmt.Sprintf("uploaded blob size %d does not match declared size %d", size, v.BlobSizeBytes))
	}

	actual, err := s.hashObject(ctx, v.BlobKey)
	if err != nil {
		return nil, fmt.Errorf("hashing blob: %w", err)
	}
	if !strings.EqualFold(actual, v.BlobSHA256) {
		return nil, s.failVariant(ctx, v, fmt.Sprintf("uploaded blob sha256 %s does not match declared %s", actual, v.BlobSHA256))
	}

	// Sign before READY: devices only ever see signed variants.
	if s.signer != nil {
		if err := s.signer.SignVariant(ctx, v.ID.String()); err != nil {
			return nil, fmt.Errorf("signing variant: %w", err)
		}
	}
	if err := s.variants.UpdateStatus(ctx, v.ID, VariantReady); err != nil {
		return nil, fmt.Errorf("marking variant ready: %w", err)
	}
	v.Status = VariantReady
	if s.onReady != nil {
		s.onReady(v.ID.String())
	}

	s.audit.Log(ctx, "system", "finalize", "model_variant", v.ID.String(), //nolint:errcheck // audit logging is best-effort
		map[string]any{"blob_key": v.BlobKey, "size": size, "sha256": actual})

	return v, nil
}

// failVariant deletes a bad blob, marks the variant FAILED and returns ErrIntegrity.
func (s *Service) failVariant(ctx context.Context, v *RegistryVariant, reason string) error {
	if err := s.storage.DeleteObject(ctx, v.BlobKey); err != nil {
		s.logger.Warn("failed to delete rejected blob", "key", v.BlobKey, "error", err)
	}
	if err := s.variants.UpdateStatus(ctx, v.ID, VariantFailed); err != nil {
		s.logger.Warn("failed to mark variant FAILED", "variant_id", v.ID, "error", err)
	}
	s.audit.Log(ctx, "system", "finalize_failed", "model_variant", v.ID.String(), //nolint:errcheck // audit logging is best-effort
		map[string]any{"reason": reason})
	return fmt.Errorf("%w: %s; the blob was deleted, upload it again", ErrIntegrity, reason)
}

func (s *Service) hashObject(ctx context.Context, key string) (string, error) {
	r, err := s.storage.OpenObject(ctx, key)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Service) DeleteVariant(ctx context.Context, id uuid.UUID) error {
	v, err := s.variants.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Delete S3 blob (best effort)
	if err := s.storage.DeleteObject(ctx, v.BlobKey); err != nil {
		s.logger.Warn("failed to delete S3 blob", "key", v.BlobKey, "error", err)
	}

	if err := s.variants.Delete(ctx, id); err != nil {
		return err
	}
	s.audit.Log(ctx, "system", "delete", "model_variant", id.String(), nil) //nolint:errcheck // audit logging is best-effort
	return nil
}

// --- Helpers ---

func clampPagination(limit, offset int) (clampedLimit, clampedOffset int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

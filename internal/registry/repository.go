package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fleetml/fleetml/internal/scope"
)

// ProjectRepository defines data access for projects.
type ProjectRepository interface {
	Create(ctx context.Context, p *Project) error
	GetByID(ctx context.Context, id uuid.UUID) (*Project, error)
	GetBySlug(ctx context.Context, slug string) (*Project, error)
	List(ctx context.Context, limit, offset int) ([]Project, int, error)
	Update(ctx context.Context, p *Project) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// ModelRepository defines data access for models.
type ModelRepository interface {
	Create(ctx context.Context, m *RegistryModel) error
	GetByID(ctx context.Context, id uuid.UUID) (*RegistryModel, error)
	ListByProject(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]RegistryModel, int, error)
	Update(ctx context.Context, m *RegistryModel) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// VersionRepository defines data access for model versions.
type VersionRepository interface {
	Create(ctx context.Context, v *RegistryVersion) error
	GetByID(ctx context.Context, id uuid.UUID) (*RegistryVersion, error)
	GetByModelAndVersion(ctx context.Context, modelID uuid.UUID, version string) (*RegistryVersion, error)
	ListByModel(ctx context.Context, modelID uuid.UUID, limit, offset int) ([]RegistryVersion, int, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, promotedBy *string) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// VariantRepository defines data access for model variants.
type VariantRepository interface {
	Create(ctx context.Context, v *RegistryVariant) error
	GetByID(ctx context.Context, id uuid.UUID) (*RegistryVariant, error)
	ListByVersion(ctx context.Context, versionID uuid.UUID) ([]RegistryVariant, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// isUniqueViolation checks if a pgx error is a unique constraint violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// --- ProjectRepo ---

type ProjectRepo struct {
	pool *pgxpool.Pool
}

func NewProjectRepo(pool *pgxpool.Pool) *ProjectRepo {
	return &ProjectRepo{pool: pool}
}

func (r *ProjectRepo) Create(ctx context.Context, p *Project) error {
	query := `
		INSERT INTO projects (name, slug, org_id, description)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query, p.Name, p.Slug, p.OrgID, p.Description).
		Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("project.Create: %w: slug %q", ErrConflict, p.Slug)
		}
		return fmt.Errorf("project.Create: %w", err)
	}
	return nil
}

func (r *ProjectRepo) GetByID(ctx context.Context, id uuid.UUID) (*Project, error) {
	query := `SELECT id, name, slug, org_id, description, created_at, updated_at FROM projects WHERE id = $1`
	p := &Project{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.Name, &p.Slug, &p.OrgID, &p.Description, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("project.GetByID: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("project.GetByID: %w", err)
	}
	return p, nil
}

func (r *ProjectRepo) GetBySlug(ctx context.Context, slug string) (*Project, error) {
	query := `SELECT id, name, slug, org_id, description, created_at, updated_at FROM projects WHERE slug = $1`
	p := &Project{}
	err := r.pool.QueryRow(ctx, query, slug).Scan(
		&p.ID, &p.Name, &p.Slug, &p.OrgID, &p.Description, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("project.GetBySlug: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("project.GetBySlug: %w", err)
	}
	return p, nil
}

func (r *ProjectRepo) List(ctx context.Context, limit, offset int) ([]Project, int, error) {
	query := `SELECT id, name, slug, org_id, description, created_at, updated_at, COUNT(*) OVER() AS total
		FROM projects WHERE ($3::uuid[] IS NULL OR id = ANY($3)) ORDER BY created_at DESC LIMIT $1 OFFSET $2`

	rows, err := r.pool.Query(ctx, query, limit, offset, scope.Projects(ctx))
	if err != nil {
		return nil, 0, fmt.Errorf("project.List: %w", err)
	}
	defer rows.Close()

	var projects []Project
	var total int
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.OrgID, &p.Description, &p.CreatedAt, &p.UpdatedAt, &total); err != nil {
			return nil, 0, fmt.Errorf("project.List: scanning row: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("project.List: %w", err)
	}
	return projects, total, nil
}

func (r *ProjectRepo) Update(ctx context.Context, p *Project) error {
	query := `UPDATE projects SET name = $1, slug = $2, org_id = $3, description = $4
		WHERE id = $5 RETURNING updated_at`

	err := r.pool.QueryRow(ctx, query, p.Name, p.Slug, p.OrgID, p.Description, p.ID).Scan(&p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("project.Update: %w", ErrNotFound)
		}
		if isUniqueViolation(err) {
			return fmt.Errorf("project.Update: %w: slug %q", ErrConflict, p.Slug)
		}
		return fmt.Errorf("project.Update: %w", err)
	}
	return nil
}

// projectVersions selects the IDs of every version of project $1.
const projectVersions = `SELECT mv.id FROM model_versions mv JOIN models m ON m.id = mv.model_id WHERE m.project_id = $1`

// Delete removes a project and everything in it, in one transaction: its
// models, versions and variants, deployments, experiments, enrollment keys and
// signing keys (by foreign key), its devices, and the telemetry, drift results
// and feature histograms of its versions.
func (r *ProjectRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("project.Delete: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, q := range []string{
		// The telemetry first, so deleting the devices does not rewrite it.
		`DELETE FROM telemetry_events WHERE model_version_id IN (` + projectVersions + `)`,
		// These keep a version ID without a foreign key.
		`DELETE FROM drift_results WHERE model_version_id IN (` + projectVersions + `)`,
		`DELETE FROM drift_points WHERE model_version_id IN (` + projectVersions + `)`,
		`DELETE FROM feature_histograms WHERE model_version_id IN (` + projectVersions + `)`,
		// Without this, the project's devices would stay, enrolled in no project.
		`DELETE FROM devices WHERE project_id = $1`,
	} {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return fmt.Errorf("project.Delete: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("project.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("project.Delete: %w", ErrNotFound)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("project.Delete: %w", err)
	}
	return nil
}

// --- ModelRepo ---

type ModelRepo struct {
	pool *pgxpool.Pool
}

func NewModelRepo(pool *pgxpool.Pool) *ModelRepo {
	return &ModelRepo{pool: pool}
}

func (r *ModelRepo) Create(ctx context.Context, m *RegistryModel) error {
	query := `
		INSERT INTO models (project_id, name, description, framework, task_type, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`

	err := r.pool.QueryRow(ctx, query, m.ProjectID, m.Name, m.Description, m.Framework, m.TaskType, m.CreatedBy).
		Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("model.Create: %w: model %q in project", ErrConflict, m.Name)
		}
		return fmt.Errorf("model.Create: %w", err)
	}
	return nil
}

func (r *ModelRepo) GetByID(ctx context.Context, id uuid.UUID) (*RegistryModel, error) {
	query := `SELECT id, project_id, name, description, framework, task_type, created_at, updated_at, created_by
		FROM models WHERE id = $1`
	m := &RegistryModel{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&m.ID, &m.ProjectID, &m.Name, &m.Description, &m.Framework, &m.TaskType,
		&m.CreatedAt, &m.UpdatedAt, &m.CreatedBy,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("model.GetByID: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("model.GetByID: %w", err)
	}
	return m, nil
}

func (r *ModelRepo) ListByProject(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]RegistryModel, int, error) {
	query := `SELECT id, project_id, name, description, framework, task_type, created_at, updated_at, created_by,
		COUNT(*) OVER() AS total
		FROM models WHERE project_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, projectID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("model.ListByProject: %w", err)
	}
	defer rows.Close()

	var models []RegistryModel
	var total int
	for rows.Next() {
		var m RegistryModel
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Name, &m.Description, &m.Framework, &m.TaskType,
			&m.CreatedAt, &m.UpdatedAt, &m.CreatedBy, &total); err != nil {
			return nil, 0, fmt.Errorf("model.ListByProject: scanning row: %w", err)
		}
		models = append(models, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("model.ListByProject: %w", err)
	}
	return models, total, nil
}

func (r *ModelRepo) Update(ctx context.Context, m *RegistryModel) error {
	query := `UPDATE models SET name = $1, description = $2, framework = $3, task_type = $4
		WHERE id = $5 RETURNING updated_at`

	err := r.pool.QueryRow(ctx, query, m.Name, m.Description, m.Framework, m.TaskType, m.ID).Scan(&m.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("model.Update: %w", ErrNotFound)
		}
		if isUniqueViolation(err) {
			return fmt.Errorf("model.Update: %w: model %q in project", ErrConflict, m.Name)
		}
		return fmt.Errorf("model.Update: %w", err)
	}
	return nil
}

func (r *ModelRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM models WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("model.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("model.Delete: %w", ErrNotFound)
	}
	return nil
}

// --- VersionRepo ---

type VersionRepo struct {
	pool *pgxpool.Pool
}

func NewVersionRepo(pool *pgxpool.Pool) *VersionRepo {
	return &VersionRepo{pool: pool}
}

func (r *VersionRepo) Create(ctx context.Context, v *RegistryVersion) error {
	metaBytes, err := json.Marshal(v.MetadataJSON)
	if err != nil {
		return fmt.Errorf("version.Create: marshalling metadata: %w", err)
	}

	query := `
		INSERT INTO model_versions (model_id, version, status, metadata_json, release_notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	err = r.pool.QueryRow(ctx, query, v.ModelID, v.Version, v.Status, metaBytes, v.ReleaseNotes).
		Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("version.Create: %w: version %q for model", ErrConflict, v.Version)
		}
		return fmt.Errorf("version.Create: %w", err)
	}
	return nil
}

func (r *VersionRepo) scanVersion(row pgx.Row) (*RegistryVersion, error) {
	v := &RegistryVersion{}
	var metaBytes []byte
	err := row.Scan(
		&v.ID, &v.ModelID, &v.Version, &v.Status, &metaBytes, &v.ReleaseNotes,
		&v.CreatedAt, &v.UpdatedAt, &v.PromotedAt, &v.PromotedBy,
	)
	if err != nil {
		return nil, err
	}
	if metaBytes != nil {
		if err := json.Unmarshal(metaBytes, &v.MetadataJSON); err != nil {
			return nil, fmt.Errorf("unmarshalling metadata: %w", err)
		}
	}
	return v, nil
}

func (r *VersionRepo) GetByID(ctx context.Context, id uuid.UUID) (*RegistryVersion, error) {
	query := `SELECT id, model_id, version, status, metadata_json, release_notes,
		created_at, updated_at, promoted_at, promoted_by
		FROM model_versions WHERE id = $1`
	v, err := r.scanVersion(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("version.GetByID: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("version.GetByID: %w", err)
	}
	return v, nil
}

func (r *VersionRepo) GetByModelAndVersion(ctx context.Context, modelID uuid.UUID, version string) (*RegistryVersion, error) {
	query := `SELECT id, model_id, version, status, metadata_json, release_notes,
		created_at, updated_at, promoted_at, promoted_by
		FROM model_versions WHERE model_id = $1 AND version = $2`
	v, err := r.scanVersion(r.pool.QueryRow(ctx, query, modelID, version))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("version.GetByModelAndVersion: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("version.GetByModelAndVersion: %w", err)
	}
	return v, nil
}

func (r *VersionRepo) ListByModel(ctx context.Context, modelID uuid.UUID, limit, offset int) ([]RegistryVersion, int, error) {
	query := `SELECT id, model_id, version, status, metadata_json, release_notes,
		created_at, updated_at, promoted_at, promoted_by, COUNT(*) OVER() AS total
		FROM model_versions WHERE model_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, modelID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("version.ListByModel: %w", err)
	}
	defer rows.Close()

	var versions []RegistryVersion
	var total int
	for rows.Next() {
		var v RegistryVersion
		var metaBytes []byte
		if err := rows.Scan(&v.ID, &v.ModelID, &v.Version, &v.Status, &metaBytes, &v.ReleaseNotes,
			&v.CreatedAt, &v.UpdatedAt, &v.PromotedAt, &v.PromotedBy, &total); err != nil {
			return nil, 0, fmt.Errorf("version.ListByModel: scanning row: %w", err)
		}
		if metaBytes != nil {
			if err := json.Unmarshal(metaBytes, &v.MetadataJSON); err != nil {
				return nil, 0, fmt.Errorf("version.ListByModel: unmarshalling metadata: %w", err)
			}
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("version.ListByModel: %w", err)
	}
	return versions, total, nil
}

func (r *VersionRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string, promotedBy *string) error {
	query := `UPDATE model_versions SET status = $1,
		promoted_at = CASE WHEN $2 THEN NOW() ELSE promoted_at END,
		promoted_by = COALESCE($3, promoted_by)
		WHERE id = $4 RETURNING updated_at`

	setPromotedAt := status == "STAGED" || status == "ACTIVE"
	var updatedAt interface{}
	err := r.pool.QueryRow(ctx, query, status, setPromotedAt, promotedBy, id).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("version.UpdateStatus: %w", ErrNotFound)
		}
		return fmt.Errorf("version.UpdateStatus: %w", err)
	}
	return nil
}

func (r *VersionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM model_versions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("version.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("version.Delete: %w", ErrNotFound)
	}
	return nil
}

// --- VariantRepo ---

type VariantRepo struct {
	pool *pgxpool.Pool
}

func NewVariantRepo(pool *pgxpool.Pool) *VariantRepo {
	return &VariantRepo{pool: pool}
}

func (r *VariantRepo) Create(ctx context.Context, v *RegistryVariant) error {
	query := `
		INSERT INTO model_variants (version_id, quantization, target_runtime, min_ram_mb, min_npu_tflops,
			blob_key, blob_size_bytes, blob_sha256, benchmark_latency_ms, benchmark_accuracy, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, COALESCE(NULLIF($11, ''), 'UPLOADING'))
		RETURNING id, status, created_at`

	err := r.pool.QueryRow(ctx, query,
		v.VersionID, v.Quantization, v.TargetRuntime, v.MinRAMMB, v.MinNPUTflops,
		v.BlobKey, v.BlobSizeBytes, v.BlobSHA256, v.BenchmarkLatencyMs, v.BenchmarkAccuracy, v.Status,
	).Scan(&v.ID, &v.Status, &v.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("variant.Create: %w: variant %s/%s for version", ErrConflict, v.Quantization, v.TargetRuntime)
		}
		return fmt.Errorf("variant.Create: %w", err)
	}
	return nil
}

func (r *VariantRepo) GetByID(ctx context.Context, id uuid.UUID) (*RegistryVariant, error) {
	query := `SELECT id, version_id, quantization, target_runtime, min_ram_mb, min_npu_tflops,
		blob_key, blob_size_bytes, blob_sha256, benchmark_latency_ms, benchmark_accuracy, status, created_at
		FROM model_variants WHERE id = $1`

	v := &RegistryVariant{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&v.ID, &v.VersionID, &v.Quantization, &v.TargetRuntime, &v.MinRAMMB, &v.MinNPUTflops,
		&v.BlobKey, &v.BlobSizeBytes, &v.BlobSHA256, &v.BenchmarkLatencyMs, &v.BenchmarkAccuracy,
		&v.Status, &v.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("variant.GetByID: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("variant.GetByID: %w", err)
	}
	return v, nil
}

func (r *VariantRepo) ListByVersion(ctx context.Context, versionID uuid.UUID) ([]RegistryVariant, error) {
	query := `SELECT id, version_id, quantization, target_runtime, min_ram_mb, min_npu_tflops,
		blob_key, blob_size_bytes, blob_sha256, benchmark_latency_ms, benchmark_accuracy, status, created_at
		FROM model_variants WHERE version_id = $1 ORDER BY created_at`

	rows, err := r.pool.Query(ctx, query, versionID)
	if err != nil {
		return nil, fmt.Errorf("variant.ListByVersion: %w", err)
	}
	defer rows.Close()

	var variants []RegistryVariant
	for rows.Next() {
		var v RegistryVariant
		if err := rows.Scan(&v.ID, &v.VersionID, &v.Quantization, &v.TargetRuntime, &v.MinRAMMB, &v.MinNPUTflops,
			&v.BlobKey, &v.BlobSizeBytes, &v.BlobSHA256, &v.BenchmarkLatencyMs, &v.BenchmarkAccuracy,
			&v.Status, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("variant.ListByVersion: scanning row: %w", err)
		}
		variants = append(variants, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("variant.ListByVersion: %w", err)
	}
	return variants, nil
}

func (r *VariantRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE model_variants SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("variant.UpdateStatus: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("variant.UpdateStatus: %w", ErrNotFound)
	}
	return nil
}

func (r *VariantRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM model_variants WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("variant.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("variant.Delete: %w", ErrNotFound)
	}
	return nil
}

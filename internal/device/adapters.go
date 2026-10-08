package device

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeploymentAdapter implements DeploymentQuerier using pgxpool.
type DeploymentAdapter struct {
	pool *pgxpool.Pool
}

// NewDeploymentAdapter creates a new DeploymentAdapter.
func NewDeploymentAdapter(pool *pgxpool.Pool) *DeploymentAdapter {
	return &DeploymentAdapter{pool: pool}
}

func (a *DeploymentAdapter) ListActiveForDevice(ctx context.Context, deviceLabels map[string]string) ([]DeploymentInfo, error) {
	labelsJSON, err := json.Marshal(deviceLabels)
	if err != nil {
		return nil, fmt.Errorf("deployment.ListActiveForDevice: marshalling labels: %w", err)
	}

	query := `SELECT id, model_version_id, strategy, COALESCE(canary_percentage, 0), status
		FROM deployments
		WHERE status IN ('PENDING', 'ROLLING_OUT', 'COMPLETE')
		AND $1::jsonb @> target_selector` // device labels must contain every selector label

	rows, err := a.pool.Query(ctx, query, labelsJSON)
	if err != nil {
		return nil, fmt.Errorf("deployment.ListActiveForDevice: %w", err)
	}
	defer rows.Close()

	var deployments []DeploymentInfo
	for rows.Next() {
		var d DeploymentInfo
		if err := rows.Scan(&d.ID, &d.ModelVersionID, &d.Strategy, &d.CanaryPct, &d.Status); err != nil {
			return nil, fmt.Errorf("deployment.ListActiveForDevice: scanning row: %w", err)
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

// VariantAdapter implements VariantQuerier using pgxpool.
type VariantAdapter struct {
	pool *pgxpool.Pool
}

// NewVariantAdapter creates a new VariantAdapter.
func NewVariantAdapter(pool *pgxpool.Pool) *VariantAdapter {
	return &VariantAdapter{pool: pool}
}

func (a *VariantAdapter) ListByVersion(ctx context.Context, versionID uuid.UUID) ([]VariantInfo, error) {
	query := `SELECT id, version_id, quantization, target_runtime, min_ram_mb, min_npu_tflops,
		blob_key, blob_size_bytes, blob_sha256
		FROM model_variants WHERE version_id = $1 AND status = 'READY'`

	rows, err := a.pool.Query(ctx, query, versionID)
	if err != nil {
		return nil, fmt.Errorf("variant.ListByVersion: %w", err)
	}
	defer rows.Close()

	var variants []VariantInfo
	for rows.Next() {
		var v VariantInfo
		if err := rows.Scan(&v.ID, &v.VersionID, &v.Quantization, &v.TargetRuntime,
			&v.MinRAMMB, &v.MinNPUTflops, &v.BlobKey, &v.BlobSizeBytes, &v.BlobSHA256); err != nil {
			return nil, fmt.Errorf("variant.ListByVersion: scanning row: %w", err)
		}
		variants = append(variants, v)
	}
	return variants, rows.Err()
}

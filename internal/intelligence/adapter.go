package intelligence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VariantRepoAdapter implements VariantRepository using pgxpool.
type VariantRepoAdapter struct {
	pool *pgxpool.Pool
}

// NewVariantRepoAdapter creates a new VariantRepoAdapter.
func NewVariantRepoAdapter(pool *pgxpool.Pool) *VariantRepoAdapter {
	return &VariantRepoAdapter{pool: pool}
}

func (a *VariantRepoAdapter) GetByVersionID(ctx context.Context, versionID uuid.UUID) ([]VariantData, error) {
	query := `SELECT mv.id, mv.version_id, mv.quantization, mv.target_runtime,
		mv.blob_key, mv.blob_size_bytes, mv.blob_sha256,
		COALESCE(ver.metadata_json, '{}'::jsonb)
		FROM model_variants mv
		JOIN model_versions ver ON ver.id = mv.version_id
		WHERE mv.version_id = $1`

	rows, err := a.pool.Query(ctx, query, versionID)
	if err != nil {
		return nil, fmt.Errorf("variantRepoAdapter.GetByVersionID: %w", err)
	}
	defer rows.Close()

	var variants []VariantData
	for rows.Next() {
		var v VariantData
		var metaJSON []byte
		if err := rows.Scan(&v.ID, &v.VersionID, &v.Quantization, &v.Runtime,
			&v.BlobKey, &v.BlobSize, &v.BlobSHA256, &metaJSON); err != nil {
			return nil, fmt.Errorf("variantRepoAdapter.GetByVersionID: scanning: %w", err)
		}
		if metaJSON != nil {
			_ = json.Unmarshal(metaJSON, &v.MetadataJSON)
		}
		variants = append(variants, v)
	}
	return variants, rows.Err()
}

// DeploymentRepoAdapter loads active deployments with targeting info.
type DeploymentRepoAdapter struct {
	pool *pgxpool.Pool
}

// NewDeploymentRepoAdapter creates a new DeploymentRepoAdapter.
func NewDeploymentRepoAdapter(pool *pgxpool.Pool) *DeploymentRepoAdapter {
	return &DeploymentRepoAdapter{pool: pool}
}

// ListActiveDeployments returns all active deployments with parsed target selectors.
// Computes effective canary percentage from canary_stages + current_stage.
func (a *DeploymentRepoAdapter) ListActiveDeployments(ctx context.Context) ([]DeploymentData, error) {
	query := `SELECT d.id, d.model_version_id, COALESCE(m.name, ''), COALESCE(mv.version, ''),
		d.target_selector, d.strategy, COALESCE(d.canary_percentage, 0),
		COALESCE(d.canary_stages, '[]'::jsonb), COALESCE(d.current_stage, 0)
		FROM deployments d
		LEFT JOIN model_versions mv ON mv.id = d.model_version_id
		LEFT JOIN models m ON m.id = mv.model_id
		WHERE d.status IN ('ROLLING_OUT', 'COMPLETE')`

	rows, err := a.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("deploymentRepoAdapter.ListActive: %w", err)
	}
	defer rows.Close()

	var deployments []DeploymentData
	for rows.Next() {
		var d DeploymentData
		var selectorJSON, stagesJSON []byte
		var currentStage int
		if err := rows.Scan(&d.ID, &d.ModelVersionID, &d.ModelName, &d.VersionString,
			&selectorJSON, &d.Strategy, &d.CanaryPercent, &stagesJSON, &currentStage); err != nil {
			return nil, fmt.Errorf("deploymentRepoAdapter.ListActive: scanning: %w", err)
		}
		selector, err := ParseTargetSelector(selectorJSON)
		if err != nil {
			// Fail closed: an unreadable selector must not fall back to "all devices".
			continue
		}
		d.TargetSelector = selector
		// Compute effective canary percent from stages
		if d.Strategy == "CANARY" && len(stagesJSON) > 2 { // not empty "[]"
			var stages []struct {
				Percentage int `json:"percentage"`
			}
			if json.Unmarshal(stagesJSON, &stages) == nil && currentStage < len(stages) {
				d.CanaryPercent = stages[currentStage].Percentage
			}
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

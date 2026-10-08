package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeviceDirectory finds the devices a deployment targets.
type DeviceDirectory interface {
	// ListMatchingDevices returns the IDs of devices whose labels contain the selector.
	ListMatchingDevices(ctx context.Context, selector map[string]string) ([]uuid.UUID, error)
	// CountUnavailable counts devices among ids that were seen since a time
	// but did not report any variant of the version loaded.
	CountUnavailable(ctx context.Context, ids []uuid.UUID, versionID uuid.UUID, seenSince time.Time) (int, error)
}

// DeviceCountAdapter implements DeviceCounter using pgxpool.
type DeviceCountAdapter struct {
	pool *pgxpool.Pool
}

// NewDeviceCountAdapter creates a new DeviceCountAdapter.
func NewDeviceCountAdapter(pool *pgxpool.Pool) *DeviceCountAdapter {
	return &DeviceCountAdapter{pool: pool}
}

func (a *DeviceCountAdapter) ListMatchingDevices(ctx context.Context, selector map[string]string) ([]uuid.UUID, error) {
	if selector == nil {
		selector = map[string]string{}
	}
	selectorJSON, err := json.Marshal(selector)
	if err != nil {
		return nil, fmt.Errorf("deviceList: marshalling: %w", err)
	}
	rows, err := a.pool.Query(ctx, `SELECT id FROM devices WHERE labels_json @> $1::jsonb`, selectorJSON)
	if err != nil {
		return nil, fmt.Errorf("deviceList: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("deviceList: scanning: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (a *DeviceCountAdapter) CountUnavailable(ctx context.Context, ids []uuid.UUID, versionID uuid.UUID, seenSince time.Time) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var n int
	err := a.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM devices d
		WHERE d.id = ANY($1) AND d.last_seen_at >= $3
		  AND NOT (d.loaded_variant_ids ?| ARRAY(SELECT id::text FROM model_variants WHERE version_id = $2))`,
		ids, versionID, seenSince).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("deviceCount.CountUnavailable: %w", err)
	}
	return n, nil
}

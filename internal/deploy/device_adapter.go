package deploy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeviceDirectory finds the devices a deployment targets.
type DeviceDirectory interface {
	CountMatchingDevices(ctx context.Context, selector map[string]string) (int, error)
	// ListMatchingDevices returns the IDs of devices whose labels contain the selector.
	ListMatchingDevices(ctx context.Context, selector map[string]string) ([]uuid.UUID, error)
}

// DeviceCountAdapter implements DeviceCounter using pgxpool.
type DeviceCountAdapter struct {
	pool *pgxpool.Pool
}

// NewDeviceCountAdapter creates a new DeviceCountAdapter.
func NewDeviceCountAdapter(pool *pgxpool.Pool) *DeviceCountAdapter {
	return &DeviceCountAdapter{pool: pool}
}

func (a *DeviceCountAdapter) CountMatchingDevices(ctx context.Context, selector map[string]string) (int, error) {
	if len(selector) == 0 {
		var count int
		err := a.pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices`).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("deviceCount: %w", err)
		}
		return count, nil
	}

	selectorJSON, err := json.Marshal(selector)
	if err != nil {
		return 0, fmt.Errorf("deviceCount: marshalling: %w", err)
	}
	var count int
	err = a.pool.QueryRow(ctx, `SELECT COUNT(*) FROM devices WHERE labels_json @> $1::jsonb`, selectorJSON).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("deviceCount: %w", err)
	}
	return count, nil
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

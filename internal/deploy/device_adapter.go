package deploy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DeviceCounter counts devices matching a target selector.
type DeviceCounter interface {
	CountMatchingDevices(ctx context.Context, selector map[string]string) (int, error)
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

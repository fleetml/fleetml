package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeviceRepository defines data access for devices.
type DeviceRepository interface {
	Create(ctx context.Context, d *Device) error
	GetByID(ctx context.Context, id uuid.UUID) (*Device, error)
	GetByFingerprint(ctx context.Context, fingerprint string) (*Device, error)
	List(ctx context.Context, limit, offset int) ([]Device, int, error)
	ListByLabels(ctx context.Context, labels map[string]string, limit, offset int) ([]Device, int, error)
	UpdateLastSeen(ctx context.Context, id uuid.UUID) error
	UpdateLabels(ctx context.Context, id uuid.UUID, labels map[string]string) error
	Update(ctx context.Context, d *Device) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// DeviceClassRepository defines data access for device classes.
type DeviceClassRepository interface {
	Create(ctx context.Context, dc *DeviceClass) error
	GetByID(ctx context.Context, id uuid.UUID) (*DeviceClass, error)
	GetByName(ctx context.Context, name string) (*DeviceClass, error)
	List(ctx context.Context) ([]DeviceClass, error)
	Upsert(ctx context.Context, dc *DeviceClass) error
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// --- DeviceRepo ---

// DeviceRepo implements DeviceRepository backed by PostgreSQL.
type DeviceRepo struct {
	pool *pgxpool.Pool
}

// NewDeviceRepo creates a new DeviceRepo.
func NewDeviceRepo(pool *pgxpool.Pool) *DeviceRepo {
	return &DeviceRepo{pool: pool}
}

func (r *DeviceRepo) Create(ctx context.Context, d *Device) error {
	labelsJSON, err := json.Marshal(d.Labels)
	if err != nil {
		return fmt.Errorf("device.Create: marshalling labels: %w", err)
	}

	query := `INSERT INTO devices (device_class_id, fingerprint, os, os_version, ram_mb, chipset,
		has_npu, has_gpu, npu_tflops, agent_version, labels_json, auth_token_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at, updated_at`

	err = r.pool.QueryRow(ctx, query,
		d.DeviceClassID, d.Fingerprint, d.OS, d.OSVersion, d.RAMMB, d.Chipset,
		d.HasNPU, d.HasGPU, d.NPUTflops, d.AgentVersion, labelsJSON, d.AuthTokenHash,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("device.Create: %w: fingerprint %q", ErrConflict, d.Fingerprint)
		}
		return fmt.Errorf("device.Create: %w", err)
	}
	return nil
}

const deviceColumns = `id, device_class_id, fingerprint, os, os_version, ram_mb, chipset,
	has_npu, has_gpu, npu_tflops, agent_version, labels_json, auth_token_hash,
	last_seen_at, created_at, updated_at`

func scanDevice(row pgx.Row) (*Device, error) {
	d := &Device{}
	var labelsJSON []byte
	err := row.Scan(
		&d.ID, &d.DeviceClassID, &d.Fingerprint, &d.OS, &d.OSVersion, &d.RAMMB, &d.Chipset,
		&d.HasNPU, &d.HasGPU, &d.NPUTflops, &d.AgentVersion, &labelsJSON, &d.AuthTokenHash,
		&d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if labelsJSON != nil {
		if err := json.Unmarshal(labelsJSON, &d.Labels); err != nil {
			return nil, fmt.Errorf("unmarshalling labels: %w", err)
		}
	}
	return d, nil
}

func (r *DeviceRepo) GetByID(ctx context.Context, id uuid.UUID) (*Device, error) {
	query := `SELECT ` + deviceColumns + ` FROM devices WHERE id = $1`
	d, err := scanDevice(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("device.GetByID: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("device.GetByID: %w", err)
	}
	return d, nil
}

func (r *DeviceRepo) GetByFingerprint(ctx context.Context, fingerprint string) (*Device, error) {
	query := `SELECT ` + deviceColumns + ` FROM devices WHERE fingerprint = $1`
	d, err := scanDevice(r.pool.QueryRow(ctx, query, fingerprint))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("device.GetByFingerprint: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("device.GetByFingerprint: %w", err)
	}
	return d, nil
}

func (r *DeviceRepo) List(ctx context.Context, limit, offset int) ([]Device, int, error) {
	query := `SELECT ` + deviceColumns + `, COUNT(*) OVER() AS total
		FROM devices ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	return r.queryDevices(ctx, query, limit, offset)
}

func (r *DeviceRepo) ListByLabels(ctx context.Context, labels map[string]string, limit, offset int) ([]Device, int, error) {
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, 0, fmt.Errorf("device.ListByLabels: marshalling labels: %w", err)
	}
	query := `SELECT ` + deviceColumns + `, COUNT(*) OVER() AS total
		FROM devices WHERE labels_json @> $1::jsonb ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, labelsJSON, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("device.ListByLabels: %w", err)
	}
	defer rows.Close()
	return scanDeviceRows(rows)
}

func (r *DeviceRepo) queryDevices(ctx context.Context, query string, limit, offset int) ([]Device, int, error) {
	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("device.List: %w", err)
	}
	defer rows.Close()
	return scanDeviceRows(rows)
}

func scanDeviceRows(rows pgx.Rows) ([]Device, int, error) {
	var devices []Device
	var total int
	for rows.Next() {
		var d Device
		var labelsJSON []byte
		if err := rows.Scan(
			&d.ID, &d.DeviceClassID, &d.Fingerprint, &d.OS, &d.OSVersion, &d.RAMMB, &d.Chipset,
			&d.HasNPU, &d.HasGPU, &d.NPUTflops, &d.AgentVersion, &labelsJSON, &d.AuthTokenHash,
			&d.LastSeenAt, &d.CreatedAt, &d.UpdatedAt, &total,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning device row: %w", err)
		}
		if labelsJSON != nil {
			if err := json.Unmarshal(labelsJSON, &d.Labels); err != nil {
				return nil, 0, fmt.Errorf("unmarshalling labels: %w", err)
			}
		}
		devices = append(devices, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return devices, total, nil
}

func (r *DeviceRepo) UpdateLastSeen(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `UPDATE devices SET last_seen_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("device.UpdateLastSeen: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("device.UpdateLastSeen: %w", ErrNotFound)
	}
	return nil
}

func (r *DeviceRepo) UpdateLabels(ctx context.Context, id uuid.UUID, labels map[string]string) error {
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return fmt.Errorf("device.UpdateLabels: marshalling labels: %w", err)
	}
	tag, err := r.pool.Exec(ctx, `UPDATE devices SET labels_json = $1 WHERE id = $2`, labelsJSON, id)
	if err != nil {
		return fmt.Errorf("device.UpdateLabels: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("device.UpdateLabels: %w", ErrNotFound)
	}
	return nil
}

func (r *DeviceRepo) Update(ctx context.Context, d *Device) error {
	labelsJSON, err := json.Marshal(d.Labels)
	if err != nil {
		return fmt.Errorf("device.Update: marshalling labels: %w", err)
	}
	query := `UPDATE devices SET device_class_id = $1, os = $2, os_version = $3, ram_mb = $4,
		chipset = $5, has_npu = $6, has_gpu = $7, npu_tflops = $8, agent_version = $9,
		labels_json = $10, auth_token_hash = $11, last_seen_at = NOW()
		WHERE id = $12 RETURNING updated_at`
	err = r.pool.QueryRow(ctx, query,
		d.DeviceClassID, d.OS, d.OSVersion, d.RAMMB, d.Chipset,
		d.HasNPU, d.HasGPU, d.NPUTflops, d.AgentVersion, labelsJSON,
		d.AuthTokenHash, d.ID,
	).Scan(&d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("device.Update: %w", ErrNotFound)
		}
		return fmt.Errorf("device.Update: %w", err)
	}
	return nil
}

func (r *DeviceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("device.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("device.Delete: %w", ErrNotFound)
	}
	return nil
}

// --- DeviceClassRepo ---

// DeviceClassRepo implements DeviceClassRepository backed by PostgreSQL.
type DeviceClassRepo struct {
	pool *pgxpool.Pool
}

// NewDeviceClassRepo creates a new DeviceClassRepo.
func NewDeviceClassRepo(pool *pgxpool.Pool) *DeviceClassRepo {
	return &DeviceClassRepo{pool: pool}
}

const classColumns = `id, name, os, min_ram_mb, has_npu, has_gpu, npu_tflops, chipset_family, created_at`

func scanClass(row pgx.Row) (*DeviceClass, error) {
	dc := &DeviceClass{}
	err := row.Scan(&dc.ID, &dc.Name, &dc.OS, &dc.MinRAMMB, &dc.HasNPU, &dc.HasGPU,
		&dc.NPUTflops, &dc.ChipsetFamily, &dc.CreatedAt)
	if err != nil {
		return nil, err
	}
	return dc, nil
}

func (r *DeviceClassRepo) Create(ctx context.Context, dc *DeviceClass) error {
	query := `INSERT INTO device_classes (name, os, min_ram_mb, has_npu, has_gpu, npu_tflops, chipset_family)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`
	err := r.pool.QueryRow(ctx, query, dc.Name, dc.OS, dc.MinRAMMB, dc.HasNPU, dc.HasGPU,
		dc.NPUTflops, dc.ChipsetFamily).Scan(&dc.ID, &dc.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("deviceClass.Create: %w: name %q", ErrConflict, dc.Name)
		}
		return fmt.Errorf("deviceClass.Create: %w", err)
	}
	return nil
}

func (r *DeviceClassRepo) GetByID(ctx context.Context, id uuid.UUID) (*DeviceClass, error) {
	query := `SELECT ` + classColumns + ` FROM device_classes WHERE id = $1`
	dc, err := scanClass(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("deviceClass.GetByID: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("deviceClass.GetByID: %w", err)
	}
	return dc, nil
}

func (r *DeviceClassRepo) GetByName(ctx context.Context, name string) (*DeviceClass, error) {
	query := `SELECT ` + classColumns + ` FROM device_classes WHERE name = $1`
	dc, err := scanClass(r.pool.QueryRow(ctx, query, name))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("deviceClass.GetByName: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("deviceClass.GetByName: %w", err)
	}
	return dc, nil
}

func (r *DeviceClassRepo) List(ctx context.Context) ([]DeviceClass, error) {
	query := `SELECT ` + classColumns + ` FROM device_classes ORDER BY name`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("deviceClass.List: %w", err)
	}
	defer rows.Close()

	var classes []DeviceClass
	for rows.Next() {
		var dc DeviceClass
		if err := rows.Scan(&dc.ID, &dc.Name, &dc.OS, &dc.MinRAMMB, &dc.HasNPU, &dc.HasGPU,
			&dc.NPUTflops, &dc.ChipsetFamily, &dc.CreatedAt); err != nil {
			return nil, fmt.Errorf("deviceClass.List: scanning row: %w", err)
		}
		classes = append(classes, dc)
	}
	return classes, rows.Err()
}

func (r *DeviceClassRepo) Upsert(ctx context.Context, dc *DeviceClass) error {
	query := `INSERT INTO device_classes (name, os, min_ram_mb, has_npu, has_gpu, npu_tflops, chipset_family)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (name) DO UPDATE SET os = EXCLUDED.os, min_ram_mb = EXCLUDED.min_ram_mb,
			has_npu = EXCLUDED.has_npu, has_gpu = EXCLUDED.has_gpu, npu_tflops = EXCLUDED.npu_tflops,
			chipset_family = EXCLUDED.chipset_family
		RETURNING id, created_at`
	err := r.pool.QueryRow(ctx, query, dc.Name, dc.OS, dc.MinRAMMB, dc.HasNPU, dc.HasGPU,
		dc.NPUTflops, dc.ChipsetFamily).Scan(&dc.ID, &dc.CreatedAt)
	if err != nil {
		return fmt.Errorf("deviceClass.Upsert: %w", err)
	}
	return nil
}

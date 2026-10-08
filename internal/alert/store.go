package alert

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned for a missing sink or project.
var ErrNotFound = errors.New("not found")

// DeliveryLogSize is how many attempts are kept per sink.
const DeliveryLogSize = 50

// Repo stores alert sinks and their delivery log in Postgres. It implements
// SinkStore and VersionLookup.
type Repo struct {
	pool *pgxpool.Pool
}

// NewRepo creates a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const sinkColumns = `id, project_id, name, kind, config, enabled, created_at`

func scanSink(row pgx.Row) (*SinkConfig, error) {
	var s SinkConfig
	if err := row.Scan(&s.ID, &s.ProjectID, &s.Name, &s.Kind, &s.Config, &s.Enabled, &s.CreatedAt); err != nil {
		return nil, err
	}
	if s.Config == nil {
		s.Config = map[string]string{}
	}
	return &s, nil
}

// Create stores a new sink; ErrNotFound if the project does not exist.
func (r *Repo) Create(ctx context.Context, s SinkConfig) (*SinkConfig, error) {
	out, err := scanSink(r.pool.QueryRow(ctx,
		`INSERT INTO alert_sinks (project_id, name, kind, config, enabled) VALUES ($1, $2, $3, $4, $5) RETURNING `+sinkColumns,
		s.ProjectID, s.Name, s.Kind, s.Config, s.Enabled))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, fmt.Errorf("alertSink.Create: project: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("alertSink.Create: %w", err)
	}
	return out, nil
}

// Get returns one sink.
func (r *Repo) Get(ctx context.Context, id string) (*SinkConfig, error) {
	s, err := scanSink(r.pool.QueryRow(ctx, `SELECT `+sinkColumns+` FROM alert_sinks WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("alertSink.Get: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("alertSink.Get: %w", err)
	}
	return s, nil
}

// List returns a project's sinks, oldest first.
func (r *Repo) List(ctx context.Context, projectID string) ([]SinkConfig, error) {
	return r.list(ctx, `SELECT `+sinkColumns+` FROM alert_sinks WHERE project_id = $1 ORDER BY created_at`, projectID)
}

// EnabledSinks returns a project's enabled sinks.
func (r *Repo) EnabledSinks(ctx context.Context, projectID string) ([]SinkConfig, error) {
	return r.list(ctx, `SELECT `+sinkColumns+` FROM alert_sinks WHERE project_id = $1 AND enabled ORDER BY created_at`, projectID)
}

func (r *Repo) list(ctx context.Context, query, projectID string) ([]SinkConfig, error) {
	rows, err := r.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("alertSink.List: %w", err)
	}
	defer rows.Close()
	out := []SinkConfig{}
	for rows.Next() {
		s, err := scanSink(rows)
		if err != nil {
			return nil, fmt.Errorf("alertSink.List: %w", err)
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Update saves a sink's name, config and enabled flag.
func (r *Repo) Update(ctx context.Context, s SinkConfig) (*SinkConfig, error) {
	out, err := scanSink(r.pool.QueryRow(ctx,
		`UPDATE alert_sinks SET name = $2, config = $3, enabled = $4, updated_at = NOW() WHERE id = $1 RETURNING `+sinkColumns,
		s.ID, s.Name, s.Config, s.Enabled))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("alertSink.Update: %w", ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("alertSink.Update: %w", err)
	}
	return out, nil
}

// Delete removes a sink and its delivery log.
func (r *Repo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM alert_sinks WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("alertSink.Delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("alertSink.Delete: %w", ErrNotFound)
	}
	return nil
}

// RecordDelivery appends an attempt and keeps the newest DeliveryLogSize per sink.
func (r *Repo) RecordDelivery(ctx context.Context, d Delivery) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO alert_deliveries (sink_id, alert_id, alert_type, attempt, status, http_status, error) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		d.SinkID, d.AlertID, d.AlertType, d.Attempt, d.Status, d.HTTPStatus, d.Error)
	if err != nil {
		return fmt.Errorf("alertDelivery.Record: %w", err)
	}
	_, err = r.pool.Exec(ctx,
		`DELETE FROM alert_deliveries WHERE sink_id = $1 AND id NOT IN
		   (SELECT id FROM alert_deliveries WHERE sink_id = $1 ORDER BY created_at DESC LIMIT $2)`,
		d.SinkID, DeliveryLogSize)
	if err != nil {
		return fmt.Errorf("alertDelivery.Prune: %w", err)
	}
	return nil
}

// Deliveries returns a sink's attempts, newest first.
func (r *Repo) Deliveries(ctx context.Context, sinkID string, limit int) ([]Delivery, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, sink_id, alert_id, alert_type, attempt, status, http_status, error, created_at
		   FROM alert_deliveries WHERE sink_id = $1 ORDER BY created_at DESC, attempt DESC LIMIT $2`, sinkID, limit)
	if err != nil {
		return nil, fmt.Errorf("alertDelivery.List: %w", err)
	}
	defer rows.Close()
	out := []Delivery{}
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.SinkID, &d.AlertID, &d.AlertType, &d.Attempt, &d.Status, &d.HTTPStatus, &d.Error, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("alertDelivery.List: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DescribeVersion implements VersionLookup.
func (r *Repo) DescribeVersion(ctx context.Context, versionID string) (projectID, modelName, version string, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT m.project_id::text, m.name, v.version FROM model_versions v JOIN models m ON m.id = v.model_id WHERE v.id = $1`,
		versionID).Scan(&projectID, &modelName, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return projectID, modelName, version, err
}

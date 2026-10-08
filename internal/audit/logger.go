// Package audit records who did what: requests that change something, refused
// requests, and the server's own actions (such as finalizing a model).
package audit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Logger struct {
	pool *pgxpool.Pool
}

func NewLogger(pool *pgxpool.Pool) *Logger {
	return &Logger{pool: pool}
}

func (l *Logger) Log(ctx context.Context, actor, action, resourceType, resourceID string, details map[string]any) error {
	query := `
		INSERT INTO audit_logs (actor, action, resource_type, resource_id, details_json)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := l.pool.Exec(ctx, query, actor, action, resourceType, resourceID, details)
	if err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}
	return nil
}

// Entry is one audit record.
type Entry struct {
	ID           int64          `json:"id"`
	RecordedAt   time.Time      `json:"recorded_at"`
	Actor        string         `json:"actor"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   uuid.UUID      `json:"resource_id"`
	ProjectID    *uuid.UUID     `json:"project_id,omitempty"`
	Status       *int           `json:"status,omitempty"`
	IP           string         `json:"ip,omitempty"`
	Details      map[string]any `json:"details"`
}

// Record stores a request's audit entry.
func (l *Logger) Record(ctx context.Context, e Entry) error {
	_, err := l.pool.Exec(ctx, `INSERT INTO audit_logs (actor, action, resource_type, resource_id, details_json, ip_address, project_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, e.Actor, e.Action, e.ResourceType, e.ResourceID, e.Details, e.IP, e.ProjectID, e.Status)
	if err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}
	return nil
}

// Filter narrows a query; zero values match everything.
type Filter struct {
	ProjectID *uuid.UUID
	Actor     string
	Action    string // a substring, e.g. "POST /deployments"
	Outcome   string // "denied" (HTTP 403) or "allowed"
	From, To  time.Time
	Limit     int
}

// Query returns matching entries, newest first.
func (l *Logger) Query(ctx context.Context, f Filter) ([]Entry, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.ProjectID != nil {
		add("project_id = $%d", *f.ProjectID)
	}
	if f.Actor != "" {
		add("actor = $%d", f.Actor)
	}
	if f.Action != "" {
		add("action ILIKE '%%' || $%d || '%%'", f.Action)
	}
	switch f.Outcome {
	case "denied":
		where = append(where, "status = 403")
	case "allowed":
		where = append(where, "(status IS NULL OR status <> 403)")
	}
	if !f.From.IsZero() {
		add("recorded_at >= $%d", f.From)
	}
	if !f.To.IsZero() {
		add("recorded_at < $%d", f.To)
	}
	q := `SELECT id, recorded_at, actor, action, resource_type, resource_id, project_id, status, COALESCE(ip_address, ''), COALESCE(details_json, '{}')
		FROM audit_logs`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	args = append(args, f.Limit)
	q += fmt.Sprintf(" ORDER BY recorded_at DESC, id DESC LIMIT $%d", len(args))
	rows, err := l.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying audit log: %w", err)
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.RecordedAt, &e.Actor, &e.Action, &e.ResourceType, &e.ResourceID, &e.ProjectID, &e.Status, &e.IP, &e.Details); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

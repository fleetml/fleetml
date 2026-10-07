package audit

import (
	"context"
	"fmt"

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

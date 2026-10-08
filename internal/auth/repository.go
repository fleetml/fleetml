package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// APIKeyRepo implements APIKeyStore backed by PostgreSQL.
type APIKeyRepo struct {
	pool *pgxpool.Pool
}

// NewAPIKeyRepo creates an APIKeyRepo.
func NewAPIKeyRepo(pool *pgxpool.Pool) *APIKeyRepo {
	return &APIKeyRepo{pool: pool}
}

const apiKeyColumns = `id, name, created_at, last_used_at, revoked_at`

func scanAPIKey(row pgx.Row) (*APIKey, error) {
	k := &APIKey{}
	if err := row.Scan(&k.ID, &k.Name, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt); err != nil {
		return nil, err
	}
	return k, nil
}

func (r *APIKeyRepo) Create(ctx context.Context, name, keyHash string) (*APIKey, error) {
	k, err := scanAPIKey(r.pool.QueryRow(ctx,
		`INSERT INTO api_keys (name, key_hash) VALUES ($1, $2) RETURNING `+apiKeyColumns, name, keyHash))
	if err != nil {
		return nil, fmt.Errorf("apiKey.Create: %w", err)
	}
	return k, nil
}

func (r *APIKeyRepo) GetActiveByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	k, err := scanAPIKey(r.pool.QueryRow(ctx,
		`SELECT `+apiKeyColumns+` FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`, keyHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("apiKey.GetActiveByHash: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("apiKey.GetActiveByHash: %w", err)
	}
	return k, nil
}

func (r *APIKeyRepo) List(ctx context.Context) ([]APIKey, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+apiKeyColumns+` FROM api_keys ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("apiKey.List: %w", err)
	}
	defer rows.Close()
	keys := []APIKey{}
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, fmt.Errorf("apiKey.List: scanning: %w", err)
		}
		keys = append(keys, *k)
	}
	return keys, rows.Err()
}

// Revoke revokes the key in one statement that also refuses to revoke the
// last active key, so concurrent revokes cannot both succeed.
func (r *APIKeyRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = NOW()
		WHERE id = $1 AND revoked_at IS NULL
		  AND (SELECT COUNT(*) FROM api_keys WHERE revoked_at IS NULL AND id <> $1) > 0`, id)
	if err != nil {
		return fmt.Errorf("apiKey.Revoke: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var revoked bool
	err = r.pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM api_keys WHERE id = $1`, id).Scan(&revoked)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("apiKey.Revoke: %w", ErrNotFound)
	case err != nil:
		return fmt.Errorf("apiKey.Revoke: %w", err)
	case revoked:
		return nil // already revoked: idempotent
	default:
		return fmt.Errorf("apiKey.Revoke: %w", ErrLastKey)
	}
}

// Touch records use, at most once a minute per key to avoid a write per request.
func (r *APIKeyRepo) Touch(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = NOW()
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < NOW() - INTERVAL '1 minute')`, id)
	if err != nil {
		return fmt.Errorf("apiKey.Touch: %w", err)
	}
	return nil
}

func (r *APIKeyRepo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM api_keys`).Scan(&n); err != nil {
		return 0, fmt.Errorf("apiKey.Count: %w", err)
	}
	return n, nil
}

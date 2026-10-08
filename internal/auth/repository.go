package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// EnrollmentKeyRepo implements EnrollmentKeyStore backed by PostgreSQL.
type EnrollmentKeyRepo struct {
	pool *pgxpool.Pool
}

// NewEnrollmentKeyRepo creates an EnrollmentKeyRepo.
func NewEnrollmentKeyRepo(pool *pgxpool.Pool) *EnrollmentKeyRepo {
	return &EnrollmentKeyRepo{pool: pool}
}

const enrollmentKeyColumns = `id, project_id, name, created_at, last_used_at, revoked_at`

func scanEnrollmentKey(row pgx.Row) (*EnrollmentKey, error) {
	k := &EnrollmentKey{}
	if err := row.Scan(&k.ID, &k.ProjectID, &k.Name, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt); err != nil {
		return nil, err
	}
	return k, nil
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func (r *EnrollmentKeyRepo) Create(ctx context.Context, projectID uuid.UUID, name, keyHash string) (*EnrollmentKey, error) {
	k, err := scanEnrollmentKey(r.pool.QueryRow(ctx,
		`INSERT INTO enrollment_keys (project_id, name, key_hash) VALUES ($1, $2, $3) RETURNING `+enrollmentKeyColumns,
		projectID, name, keyHash))
	if err != nil {
		if isForeignKeyViolation(err) {
			return nil, fmt.Errorf("enrollmentKey.Create: project: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("enrollmentKey.Create: %w", err)
	}
	return k, nil
}

func (r *EnrollmentKeyRepo) GetActiveByHash(ctx context.Context, keyHash string) (*EnrollmentKey, error) {
	k, err := scanEnrollmentKey(r.pool.QueryRow(ctx,
		`SELECT `+enrollmentKeyColumns+` FROM enrollment_keys WHERE key_hash = $1 AND revoked_at IS NULL`, keyHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("enrollmentKey.GetActiveByHash: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("enrollmentKey.GetActiveByHash: %w", err)
	}
	return k, nil
}

func (r *EnrollmentKeyRepo) ListByProject(ctx context.Context, projectID uuid.UUID) ([]EnrollmentKey, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+enrollmentKeyColumns+` FROM enrollment_keys WHERE project_id = $1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("enrollmentKey.ListByProject: %w", err)
	}
	defer rows.Close()
	keys := []EnrollmentKey{}
	for rows.Next() {
		k, err := scanEnrollmentKey(rows)
		if err != nil {
			return nil, fmt.Errorf("enrollmentKey.ListByProject: scanning: %w", err)
		}
		keys = append(keys, *k)
	}
	return keys, rows.Err()
}

func (r *EnrollmentKeyRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE enrollment_keys SET revoked_at = COALESCE(revoked_at, NOW()) WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("enrollmentKey.Revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("enrollmentKey.Revoke: %w", ErrNotFound)
	}
	return nil
}

func (r *EnrollmentKeyRepo) Rotate(ctx context.Context, id uuid.UUID, newKeyHash string) (*EnrollmentKey, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("enrollmentKey.Rotate: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var projectID uuid.UUID
	var name string
	err = tx.QueryRow(ctx,
		`UPDATE enrollment_keys SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL RETURNING project_id, name`,
		id).Scan(&projectID, &name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("enrollmentKey.Rotate: active key: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("enrollmentKey.Rotate: %w", err)
	}
	k, err := scanEnrollmentKey(tx.QueryRow(ctx,
		`INSERT INTO enrollment_keys (project_id, name, key_hash) VALUES ($1, $2, $3) RETURNING `+enrollmentKeyColumns,
		projectID, name, newKeyHash))
	if err != nil {
		return nil, fmt.Errorf("enrollmentKey.Rotate: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("enrollmentKey.Rotate: commit: %w", err)
	}
	return k, nil
}

func (r *EnrollmentKeyRepo) Touch(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE enrollment_keys SET last_used_at = NOW()
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < NOW() - INTERVAL '1 minute')`, id)
	if err != nil {
		return fmt.Errorf("enrollmentKey.Touch: %w", err)
	}
	return nil
}

// SessionRepo implements SessionStore backed by PostgreSQL.
type SessionRepo struct {
	pool *pgxpool.Pool
}

// NewSessionRepo creates a SessionRepo.
func NewSessionRepo(pool *pgxpool.Pool) *SessionRepo {
	return &SessionRepo{pool: pool}
}

const sessionColumns = `s.id, s.api_key_id, k.name, s.user_agent, s.ip, s.created_at, s.last_seen_at, s.expires_at, s.revoked_at`

func scanSession(row pgx.Row) (*Session, error) {
	s := &Session{}
	if err := row.Scan(&s.ID, &s.APIKeyID, &s.APIKeyName, &s.UserAgent, &s.IP, &s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt, &s.RevokedAt); err != nil {
		return nil, err
	}
	return s, nil
}

func (r *SessionRepo) Create(ctx context.Context, apiKeyID uuid.UUID, tokenHash, userAgent, ip string, expiresAt time.Time) (*Session, error) {
	// Sessions expired for a week are of no further interest.
	if _, err := r.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW() - INTERVAL '7 days'`); err != nil {
		return nil, fmt.Errorf("session.Create: pruning: %w", err)
	}
	s, err := scanSession(r.pool.QueryRow(ctx,
		`WITH s AS (INSERT INTO sessions (api_key_id, token_hash, user_agent, ip, expires_at) VALUES ($1, $2, $3, $4, $5) RETURNING *)
		SELECT `+sessionColumns+` FROM s JOIN api_keys k ON k.id = s.api_key_id`,
		apiKeyID, tokenHash, userAgent, ip, expiresAt))
	if err != nil {
		return nil, fmt.Errorf("session.Create: %w", err)
	}
	return s, nil
}

const activeSession = `s.revoked_at IS NULL AND s.expires_at > NOW() AND k.revoked_at IS NULL`

func (r *SessionRepo) GetActiveByHash(ctx context.Context, tokenHash string) (*Session, error) {
	s, err := scanSession(r.pool.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM sessions s JOIN api_keys k ON k.id = s.api_key_id WHERE s.token_hash = $1 AND `+activeSession, tokenHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("session.GetActiveByHash: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("session.GetActiveByHash: %w", err)
	}
	return s, nil
}

func (r *SessionRepo) ListActive(ctx context.Context) ([]Session, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+sessionColumns+` FROM sessions s JOIN api_keys k ON k.id = s.api_key_id WHERE `+activeSession+` ORDER BY s.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("session.ListActive: %w", err)
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("session.ListActive: %w", err)
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Revoke is idempotent for a revoked session; ErrNotFound for an unknown one.
func (r *SessionRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `UPDATE sessions SET revoked_at = COALESCE(revoked_at, NOW()) WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("session.Revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("session.Revoke: %w", ErrNotFound)
	}
	return nil
}

func (r *SessionRepo) Touch(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sessions SET last_seen_at = NOW() WHERE id = $1 AND last_seen_at < NOW() - INTERVAL '1 minute'`, id)
	if err != nil {
		return fmt.Errorf("session.Touch: %w", err)
	}
	return nil
}

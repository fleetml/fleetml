// Package dbtest gives integration tests a throwaway, fully migrated
// database on the dev stack's Postgres/TimescaleDB.
//
// Tests that use it carry the "integration" build tag and are skipped unless
// FLEETML_TEST_DATABASE_URL points at a server where the user may create
// databases, e.g. the dev stack:
//
//	FLEETML_TEST_DATABASE_URL=postgres://fleetml:fleetml_dev@localhost:15432/fleetml?sslmode=disable \
//	  go test -tags integration ./...
package dbtest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fleetml/fleetml/internal/db"
)

// EnvURL names the environment variable with the admin connection URL.
const EnvURL = "FLEETML_TEST_DATABASE_URL"

// Pool creates a new database, applies every migration, and returns a pool
// to it. The database is dropped when the test finishes.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	adminURL := os.Getenv(EnvURL)
	if adminURL == "" {
		t.Skipf("%s not set; start the dev stack and set it to run integration tests", EnvURL)
	}
	ctx := context.Background()

	name := "fleetml_test_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("connecting to %s: %v", EnvURL, err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("creating test database: %v", err)
	}
	_ = admin.Close(ctx)

	testURL, err := withDatabase(adminURL, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(testURL); err != nil {
		t.Fatalf("migrating test database: %v", err)
	}
	pool, err := db.NewPool(ctx, testURL)
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		admin, err := pgx.Connect(context.Background(), adminURL)
		if err != nil {
			return
		}
		defer func() { _ = admin.Close(context.Background()) }()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return pool
}

func withDatabase(rawURL, name string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing %s: %w", EnvURL, err)
	}
	u.Path = "/" + name
	return u.String(), nil
}

// Fixture holds IDs created by NewVersion.
type Fixture struct {
	ProjectID, ModelID, VersionID, VariantID uuid.UUID
}

// NewVersion inserts a project, a model, a version and one READY INT8 TFLITE
// variant. Call it again with the same fixture's ModelID via NewVersionOf to
// add versions of the same model.
func NewVersion(t *testing.T, pool *pgxpool.Pool) Fixture {
	t.Helper()
	ctx := context.Background()
	var f Fixture
	slug := "p-" + uuid.NewString()[:8]
	must(t, pool.QueryRow(ctx, `INSERT INTO projects (name, slug) VALUES ($1, $1) RETURNING id`, slug).Scan(&f.ProjectID))
	must(t, pool.QueryRow(ctx, `INSERT INTO models (project_id, name, framework, task_type) VALUES ($1, 'm', 'TFLITE', 'CLASSIFICATION') RETURNING id`,
		f.ProjectID).Scan(&f.ModelID))
	f.VersionID, f.VariantID = NewVersionOf(t, pool, f.ModelID, "1.0.0")
	return f
}

// NewVersionOf adds a version with one READY variant to an existing model.
func NewVersionOf(t *testing.T, pool *pgxpool.Pool, modelID uuid.UUID, version string) (versionID, variantID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	must(t, pool.QueryRow(ctx, `INSERT INTO model_versions (model_id, version) VALUES ($1, $2) RETURNING id`, modelID, version).Scan(&versionID))
	must(t, pool.QueryRow(ctx, `INSERT INTO model_variants (version_id, quantization, target_runtime, blob_key, blob_size_bytes, blob_sha256, status)
		VALUES ($1, 'INT8', 'TFLITE', $2, 1024, 'sha', 'READY') RETURNING id`, versionID, "k/"+versionID.String()).Scan(&variantID))
	return versionID, variantID
}

// NewDevice inserts a device with the given labels and returns its ID.
func NewDevice(t *testing.T, pool *pgxpool.Pool, labels string) uuid.UUID {
	t.Helper()
	if labels == "" {
		labels = "{}"
	}
	var id uuid.UUID
	must(t, pool.QueryRow(context.Background(),
		`INSERT INTO devices (fingerprint, os, ram_mb, labels_json, last_seen_at) VALUES ($1, 'ANDROID', 4096, $2::jsonb, NOW()) RETURNING id`,
		strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")[:64], labels).Scan(&id))
	return id
}

// NewProjectDevice inserts a device enrolled into projectID with the given
// labels and returns its ID.
func NewProjectDevice(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, labels string) uuid.UUID {
	t.Helper()
	id := NewDevice(t, pool, labels)
	_, err := pool.Exec(context.Background(), `UPDATE devices SET project_id = $2 WHERE id = $1`, id, projectID)
	must(t, err)
	return id
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

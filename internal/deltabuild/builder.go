// Package deltabuild builds model deltas on the server: when a variant becomes
// READY, it diffs it against the previous READY variant of the same model,
// runtime and quantization, stores the delta in the object store, and offers
// it to devices that have the base.
package deltabuild

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fleetml/fleetml/internal/delta"
)

// Storage is the object store the builder reads models from and writes deltas to.
type Storage interface {
	OpenObject(ctx context.Context, key string) (io.ReadCloser, error)
	PutObject(ctx context.Context, key string, data []byte) error
	GeneratePresignedDownload(ctx context.Context, key string, expiry time.Duration) (string, error)
}

// Delta is a delta a device can download instead of the full model.
type Delta struct {
	URL        string
	Size       int64
	SHA256     string
	BaseSHA256 string
}

// Builder builds and looks up deltas.
type Builder struct {
	pool     *pgxpool.Pool
	storage  Storage
	maxBytes int64
	logger   *slog.Logger
	sem      chan struct{} // one build at a time: a diff holds both files and ~8 bytes per byte of the base
	wg       sync.WaitGroup
}

// New creates a Builder; models above maxBytes get no delta (0 disables deltas).
func New(pool *pgxpool.Pool, storage Storage, maxBytes int64, logger *slog.Logger) *Builder {
	return &Builder{pool: pool, storage: storage, maxBytes: maxBytes, logger: logger, sem: make(chan struct{}, 1)}
}

// BuildFor builds, in the background, the delta to a variant that just became READY.
func (b *Builder) BuildFor(variantID string) {
	if b.maxBytes <= 0 {
		return
	}
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		b.sem <- struct{}{}
		defer func() { <-b.sem }()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := b.build(ctx, variantID); err != nil {
			b.logger.Warn("building a model delta failed; devices download the full model", "variant_id", variantID, "error", err)
		}
	}()
}

// Wait blocks until running builds finish.
func (b *Builder) Wait() { b.wg.Wait() }

func (b *Builder) build(ctx context.Context, targetID string) error {
	var modelID, versionID, quant, runtime, targetKey string
	var targetSize int64
	var created time.Time
	err := b.pool.QueryRow(ctx, `SELECT ver.model_id::text, ver.id::text, mv.quantization, mv.target_runtime, mv.blob_key, mv.blob_size_bytes, mv.created_at
		FROM model_variants mv JOIN model_versions ver ON ver.id = mv.version_id WHERE mv.id = $1 AND mv.status = 'READY'`, targetID).
		Scan(&modelID, &versionID, &quant, &runtime, &targetKey, &targetSize, &created)
	if err != nil {
		return fmt.Errorf("loading the variant: %w", err)
	}
	var baseID, baseKey string
	var baseSize int64
	err = b.pool.QueryRow(ctx, `SELECT mv.id::text, mv.blob_key, mv.blob_size_bytes
		FROM model_variants mv JOIN model_versions ver ON ver.id = mv.version_id
		WHERE ver.model_id = $1 AND ver.id <> $2 AND mv.quantization = $3 AND mv.target_runtime = $4
		  AND mv.status = 'READY' AND mv.created_at < $5
		ORDER BY mv.created_at DESC LIMIT 1`, modelID, versionID, quant, runtime, created).Scan(&baseID, &baseKey, &baseSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // the model's first version: nothing to diff against
	}
	if err != nil {
		return fmt.Errorf("finding the previous variant: %w", err)
	}
	if targetSize > b.maxBytes || baseSize > b.maxBytes {
		b.logger.Info("no delta: the model is larger than DELTA_MAX_SIZE_MB", "variant_id", targetID, "size", targetSize)
		return nil
	}
	start := time.Now()
	base, err := b.read(ctx, baseKey)
	if err != nil {
		return err
	}
	target, err := b.read(ctx, targetKey)
	if err != nil {
		return err
	}
	d, err := delta.Diff(base, target)
	if err != nil {
		return err
	}
	if int64(len(d)) >= targetSize/2 {
		b.logger.Info("no delta: it would be more than half of the model", "variant_id", targetID, "delta_bytes", len(d), "model_bytes", targetSize)
		return nil
	}
	key := "deltas/" + targetID + "/" + baseID + ".fmldelta"
	if err := b.storage.PutObject(ctx, key, d); err != nil {
		return err
	}
	sum := sha256.Sum256(d)
	_, err = b.pool.Exec(ctx, `INSERT INTO variant_deltas (base_variant_id, target_variant_id, blob_key, size_bytes, sha256)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`, baseID, targetID, key, len(d), hex.EncodeToString(sum[:]))
	if err != nil {
		return fmt.Errorf("recording the delta: %w", err)
	}
	b.logger.Info("model delta built", "base_variant_id", baseID, "variant_id", targetID,
		"delta_bytes", len(d), "model_bytes", targetSize, "took", time.Since(start).Round(time.Millisecond))
	return nil
}

func (b *Builder) read(ctx context.Context, key string) ([]byte, error) {
	r, err := b.storage.OpenObject(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}

// Lookup returns the delta from base to target with a download URL, or nil if there is none.
func (b *Builder) Lookup(ctx context.Context, baseVariantID, targetVariantID string) (*Delta, error) {
	var key string
	d := &Delta{}
	err := b.pool.QueryRow(ctx, `SELECT d.blob_key, d.size_bytes, d.sha256, base.blob_sha256
		FROM variant_deltas d JOIN model_variants base ON base.id = d.base_variant_id
		WHERE d.base_variant_id = $1 AND d.target_variant_id = $2`, baseVariantID, targetVariantID).Scan(&key, &d.Size, &d.SHA256, &d.BaseSHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("looking up a delta: %w", err)
	}
	if d.URL, err = b.storage.GeneratePresignedDownload(ctx, key, time.Hour); err != nil {
		return nil, err
	}
	return d, nil
}

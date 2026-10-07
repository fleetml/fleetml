package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nats-io/nats.go"
)

const (
	defaultBatchSize     = 500
	defaultFlushInterval = 5 * time.Second
	finalFlushTimeout    = 10 * time.Second
)

// eventWriter is the subset of Repository the consumer needs.
type eventWriter interface {
	InsertEvents(ctx context.Context, events []Event) (int, error)
	InsertEvent(ctx context.Context, e Event) error
}

// Consumer subscribes to NATS and writes telemetry to the database in batches.
type Consumer struct {
	nc            *nats.Conn
	repo          eventWriter
	logger        *slog.Logger
	batchSize     int
	flushInterval time.Duration

	mu      sync.Mutex
	buffer  []Event
	sub     *nats.Subscription
	dropped atomic.Int64
}

// NewConsumer creates a new telemetry consumer.
func NewConsumer(nc *nats.Conn, repo *Repository, logger *slog.Logger) *Consumer {
	return &Consumer{
		nc:            nc,
		repo:          repo,
		logger:        logger,
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
	}
}

// Dropped returns the number of events discarded because the database
// rejected them (e.g. unknown model_version_id) or they were invalid.
func (c *Consumer) Dropped() int64 {
	return c.dropped.Load()
}

// Start begins consuming telemetry events from NATS.
func (c *Consumer) Start(ctx context.Context) error {
	sub, err := c.nc.Subscribe(natsSubjectTelemetry, func(msg *nats.Msg) {
		var events []Event
		if err := json.Unmarshal(msg.Data, &events); err != nil {
			c.logger.Warn("telemetry consumer: failed to unmarshal", "error", err)
			return
		}
		c.addToBuffer(ctx, events)
	})
	if err != nil {
		return err
	}
	c.sub = sub

	// Periodic flush
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.flush(ctx)
		case <-ctx.Done():
			c.finalFlush()
			return nil
		}
	}
}

// Stop gracefully shuts down the consumer.
func (c *Consumer) Stop() error {
	if c.sub != nil {
		return c.sub.Unsubscribe()
	}
	return nil
}

// finalFlush drains the buffer on shutdown. The consumer's context is already
// cancelled at that point, so it uses a fresh one.
func (c *Consumer) finalFlush() {
	ctx, cancel := context.WithTimeout(context.Background(), finalFlushTimeout)
	defer cancel()
	c.flush(ctx)
}

func (c *Consumer) addToBuffer(ctx context.Context, events []Event) {
	c.mu.Lock()
	c.buffer = append(c.buffer, events...)
	shouldFlush := len(c.buffer) >= c.batchSize
	c.mu.Unlock()

	if shouldFlush {
		c.flush(ctx)
	}
}

func (c *Consumer) flush(ctx context.Context) {
	c.mu.Lock()
	if len(c.buffer) == 0 {
		c.mu.Unlock()
		return
	}
	batch := c.buffer
	c.buffer = nil
	c.mu.Unlock()

	batch = c.dropInvalid(batch)
	if len(batch) == 0 {
		return
	}

	count, err := c.repo.InsertEvents(ctx, batch)
	if err == nil {
		c.logger.Info("telemetry consumer: flushed events to database", "count", count)
		return
	}

	// A single bad row aborts the whole COPY. Fall back to row-by-row inserts
	// so only the offending rows are discarded.
	c.logger.Warn("telemetry consumer: batch insert failed, retrying row by row", "error", err, "batch_size", len(batch))
	inserted, retry := c.insertRowByRow(ctx, batch)
	if len(retry) > 0 {
		// Transient (non-Postgres) failures, e.g. the database is unreachable: keep for the next flush.
		c.mu.Lock()
		c.buffer = append(retry, c.buffer...)
		c.mu.Unlock()
	}
	c.logger.Info("telemetry consumer: row-by-row flush complete",
		"inserted", inserted, "requeued", len(retry), "dropped_total", c.dropped.Load())
}

// dropInvalid removes events that can never be stored.
func (c *Consumer) dropInvalid(batch []Event) []Event {
	valid := batch[:0]
	for _, e := range batch {
		if e.ModelVersionID == uuid.Nil {
			c.dropped.Add(1)
			c.logger.Warn("telemetry consumer: dropping event without model_version_id",
				"device_id", e.DeviceID, "dropped_total", c.dropped.Load())
			continue
		}
		valid = append(valid, e)
	}
	return valid
}

func (c *Consumer) insertRowByRow(ctx context.Context, batch []Event) (inserted int, retry []Event) {
	for _, e := range batch {
		err := c.repo.InsertEvent(ctx, e)
		if err == nil {
			inserted++
			continue
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			c.dropped.Add(1)
			c.logger.Warn("telemetry consumer: dropping event rejected by database",
				"device_id", e.DeviceID, "model_version_id", e.ModelVersionID,
				"sqlstate", pgErr.Code, "error", pgErr.Message, "dropped_total", c.dropped.Load())
			continue
		}
		retry = append(retry, e)
	}
	return inserted, retry
}

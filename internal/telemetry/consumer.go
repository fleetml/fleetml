package telemetry

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	defaultBatchSize     = 500
	defaultFlushInterval = 5 * time.Second
)

// Consumer subscribes to NATS and writes telemetry to the database in batches.
type Consumer struct {
	nc            *nats.Conn
	repo          *Repository
	logger        *slog.Logger
	batchSize     int
	flushInterval time.Duration

	mu     sync.Mutex
	buffer []Event
	sub    *nats.Subscription
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
			c.flush(ctx) // Final flush
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

	count, err := c.repo.InsertEvents(ctx, batch)
	if err != nil {
		c.logger.Error("telemetry consumer: flush failed", "error", err, "batch_size", len(batch))
		// Re-add to buffer for retry
		c.mu.Lock()
		c.buffer = append(batch, c.buffer...)
		c.mu.Unlock()
		return
	}
	c.logger.Info("telemetry consumer: flushed events to database", "count", count)
}

package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	// StreamName is the JetStream stream that holds accepted telemetry until
	// it is in the database; ConsumerName is its durable consumer.
	StreamName   = "FLEETML_TELEMETRY"
	ConsumerName = "telemetry-writer"

	defaultFetchBatch   = 50 // messages, each one device batch of up to 1000 events
	defaultFetchMaxWait = 2 * time.Second
	writeTimeout        = 30 * time.Second
	retryDelay          = 5 * time.Second
	lagInterval         = 5 * time.Second
)

// eventWriter is the subset of Repository the consumer needs.
type eventWriter interface {
	// InsertEvents stores events, skipping ones already stored (same event ID); it returns how many were new.
	InsertEvents(ctx context.Context, events []Event) (int, error)
	InsertEvent(ctx context.Context, e Event) (bool, error)
}

// Consumer moves telemetry from the JetStream stream into the database. A
// message is acknowledged only after its events are committed, so a crash
// redelivers it; inserts are idempotent on the event ID, so redelivery never
// stores an event twice.
type Consumer struct {
	js         jetstream.JetStream
	repo       eventWriter
	logger     *slog.Logger
	fetchBatch int
	fetchWait  time.Duration

	dropped     atomic.Int64
	redelivered atomic.Int64
	duplicates  atomic.Int64
	pending     atomic.Int64
}

// NewConsumer creates a telemetry consumer.
func NewConsumer(js jetstream.JetStream, repo *Repository, logger *slog.Logger) *Consumer {
	return &Consumer{js: js, repo: repo, logger: logger, fetchBatch: defaultFetchBatch, fetchWait: defaultFetchMaxWait}
}

// Dropped is the number of events discarded because the database rejected
// them (e.g. an unknown model version) or they were invalid.
func (c *Consumer) Dropped() int64 { return c.dropped.Load() }

// Redelivered is the number of messages JetStream delivered more than once.
func (c *Consumer) Redelivered() int64 { return c.redelivered.Load() }

// Duplicates is the number of events skipped because they were already stored.
func (c *Consumer) Duplicates() int64 { return c.duplicates.Load() }

// Pending is the consumer's lag: messages not yet delivered or not yet acknowledged.
func (c *Consumer) Pending() int64 { return c.pending.Load() }

// EnsureStream creates or updates the telemetry stream: file storage, work
// queue retention (a message is removed once acknowledged), and a duplicate
// window that drops a batch published twice.
func EnsureStream(ctx context.Context, js jetstream.JetStream) (jetstream.Stream, error) {
	return js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       StreamName,
		Subjects:   []string{Subject},
		Retention:  jetstream.WorkQueuePolicy,
		Storage:    jetstream.FileStorage,
		MaxAge:     7 * 24 * time.Hour,
		Duplicates: 2 * time.Minute,
	})
}

// Start consumes until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context) error {
	stream, err := EnsureStream(ctx, c.js)
	if err != nil {
		return err
	}
	cons, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       ConsumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       writeTimeout + 10*time.Second,
		MaxAckPending: 4 * defaultFetchBatch,
	})
	if err != nil {
		return err
	}
	go c.watchLag(ctx, cons)
	for ctx.Err() == nil {
		batch, err := cons.Fetch(c.fetchBatch, jetstream.FetchMaxWait(c.fetchWait))
		if err != nil {
			if ctx.Err() == nil {
				c.logger.Warn("telemetry consumer: fetch failed", "error", err)
				time.Sleep(time.Second)
			}
			continue
		}
		var msgs []jetstream.Msg
		var events []Event
		for msg := range batch.Messages() {
			if meta, err := msg.Metadata(); err == nil && meta.NumDelivered > 1 {
				c.redelivered.Add(1)
			}
			var evs []Event
			if err := json.Unmarshal(msg.Data(), &evs); err != nil {
				c.logger.Warn("telemetry consumer: dropping a message that is not an event batch", "error", err)
				_ = msg.Term()
				continue
			}
			msgs = append(msgs, msg)
			events = append(events, evs...)
		}
		if len(msgs) == 0 {
			continue
		}
		// The batch is written even during shutdown; unacknowledged messages
		// would be redelivered anyway.
		if c.write(ctx, events) {
			for _, m := range msgs {
				_ = m.Ack()
			}
		} else {
			for _, m := range msgs {
				_ = m.NakWithDelay(retryDelay)
			}
		}
	}
	return nil
}

func (c *Consumer) watchLag(ctx context.Context, cons jetstream.Consumer) {
	t := time.NewTicker(lagInterval)
	defer t.Stop()
	for {
		if info, err := cons.Info(ctx); err == nil {
			c.pending.Store(int64(info.NumPending) + int64(info.NumAckPending))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// write stores a batch and reports whether its messages may be acknowledged:
// true when every event is stored, already stored, or rejected for good; false
// on a transient failure (the messages are then redelivered).
func (c *Consumer) write(ctx context.Context, batch []Event) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()
	batch = c.dropInvalid(batch)
	if len(batch) == 0 {
		return true
	}
	inserted, err := c.repo.InsertEvents(ctx, batch)
	if err == nil {
		c.duplicates.Add(int64(len(batch) - inserted))
		c.logger.Info("telemetry consumer: stored events", "count", inserted, "duplicates", len(batch)-inserted)
		return true
	}

	// A single bad row aborts the whole batch. Fall back to row-by-row
	// inserts so only the offending rows are discarded.
	c.logger.Warn("telemetry consumer: batch insert failed, retrying row by row", "error", err, "batch_size", len(batch))
	for _, e := range batch {
		stored, err := c.repo.InsertEvent(ctx, e)
		if err == nil {
			if !stored {
				c.duplicates.Add(1)
			}
			continue
		}
		if pgErr := rowError(err); pgErr != nil {
			c.dropped.Add(1)
			c.logger.Warn("telemetry consumer: dropping event rejected by database",
				"device_id", e.DeviceID, "model_version_id", e.ModelVersionID,
				"sqlstate", pgErr.Code, "error", pgErr.Message, "dropped_total", c.dropped.Load())
			continue
		}
		// Anything else (the database is unreachable, a missing migration, ...)
		// is retried with the whole batch; rows already stored are skipped then.
		c.logger.Warn("telemetry consumer: storing failed, will retry", "error", err)
		return false
	}
	return true
}

// dropInvalid removes events that can never be stored.
func (c *Consumer) dropInvalid(batch []Event) []Event {
	valid := batch[:0:0]
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

// rowError returns the Postgres error if err means this row can never be
// stored: a data exception (class 22, e.g. an invalid value) or a violated
// constraint (class 23, e.g. an unknown model version). Other errors, such as
// a schema without a migration, are not the row's fault and are retried.
func rowError(err error) *pgconn.PgError {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (strings.HasPrefix(pgErr.Code, "22") || strings.HasPrefix(pgErr.Code, "23")) {
		return pgErr
	}
	return nil
}

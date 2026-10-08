package telemetry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go/jetstream"
)

// Subject carries accepted telemetry batches into the JetStream stream.
const Subject = "fleetml.telemetry.events"

// Publisher puts accepted telemetry into the JetStream stream. Publish returns
// once the stream has stored the batch, so a device's events are durable
// before ReportBatch acknowledges them.
type Publisher struct {
	js     jetstream.JetStream
	logger *slog.Logger
}

// NewPublisher creates a telemetry publisher.
func NewPublisher(js jetstream.JetStream, logger *slog.Logger) *Publisher {
	return &Publisher{js: js, logger: logger}
}

// Publish stores a batch in the stream. The message ID is derived from the
// event IDs, so the same batch published twice within two minutes is stored
// once.
func (p *Publisher) Publish(ctx context.Context, events []Event) error {
	data, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("telemetry.Publisher: marshalling: %w", err)
	}
	h := sha256.New()
	for _, e := range events {
		h.Write(e.EventID[:])
	}
	if _, err := p.js.Publish(ctx, Subject, data, jetstream.WithMsgID(hex.EncodeToString(h.Sum(nil)))); err != nil {
		return fmt.Errorf("telemetry.Publisher: publishing to JetStream: %w", err)
	}
	p.logger.Debug("published telemetry to JetStream", "event_count", len(events))
	return nil
}

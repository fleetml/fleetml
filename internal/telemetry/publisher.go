package telemetry

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
)

const natsSubjectTelemetry = "fleetml.telemetry.events"

// Publisher publishes telemetry events to NATS for async processing.
type Publisher struct {
	nc     *nats.Conn
	logger *slog.Logger
}

// NewPublisher creates a new telemetry publisher.
func NewPublisher(nc *nats.Conn, logger *slog.Logger) *Publisher {
	return &Publisher{nc: nc, logger: logger}
}

// Publish serializes and publishes a batch of events to NATS.
func (p *Publisher) Publish(events []Event) error {
	data, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("telemetry.Publisher: marshalling: %w", err)
	}

	if err := p.nc.Publish(natsSubjectTelemetry, data); err != nil {
		return fmt.Errorf("telemetry.Publisher: publishing to NATS: %w", err)
	}

	p.logger.Debug("published telemetry to NATS", "event_count", len(events))
	return nil
}

// IsConnected returns true if the NATS connection is alive.
func (p *Publisher) IsConnected() bool {
	return p.nc.IsConnected()
}

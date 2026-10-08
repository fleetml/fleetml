package experiment

import (
	"context"
	"log/slog"
	"time"

	"github.com/fleetml/fleetml/internal/metrics"
)

// Ticker periodically evaluates running experiments.
type Ticker struct {
	service  *Service
	interval time.Duration
	logger   *slog.Logger
	stopCh   chan struct{}
}

// NewTicker creates a new experiment ticker.
func NewTicker(service *Service, interval time.Duration, logger *slog.Logger) *Ticker {
	return &Ticker{service: service, interval: interval, logger: logger, stopCh: make(chan struct{})}
}

// Start begins the ticker loop.
func (t *Ticker) Start(ctx context.Context) {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.tick(ctx)
		}
	}
}

// Stop stops the ticker.
func (t *Ticker) Stop() {
	select {
	case <-t.stopCh:
	default:
		close(t.stopCh)
	}
}

func (t *Ticker) tick(ctx context.Context) {
	defer metrics.ObserveTick("experiment", time.Now())
	exps, err := t.service.ListRunning(ctx)
	if err != nil {
		if t.logger != nil {
			t.logger.Error("experiment ticker: failed to list", "error", err)
		}
		return
	}

	for _, exp := range exps {
		if exp.Expired(time.Now()) {
			t.concludeExpired(ctx, &exp)
			continue
		}
		result, err := t.service.EvaluateExperiment(ctx, exp.ID)
		if err != nil {
			if t.logger != nil {
				t.logger.Warn("experiment ticker: evaluation failed", "experiment_id", exp.ID, "error", err)
			}
			continue
		}

		// Auto-conclude at the fixed horizon: the single test has run.
		if exp.AutoConclude && result.Final {
			winner := exp.ControlVersionID
			note := "auto-concluded at the fixed horizon: no significant difference, keeping control"
			if result.IsSignificant {
				note = "auto-concluded at the fixed horizon: " + result.Recommendation
				if result.Recommendation == "ADOPT_TREATMENT" {
					winner = exp.TreatmentVersionID
				}
			}
			if _, err := t.service.Conclude(ctx, exp.ID, winner, note); err != nil {
				if t.logger != nil {
					t.logger.Warn("experiment ticker: auto-conclude failed", "experiment_id", exp.ID, "error", err)
				}
			} else if t.logger != nil {
				t.logger.Info("experiment auto-concluded", "experiment_id", exp.ID, "winner", winner, "recommendation", result.Recommendation)
			}
		}
	}
}

// concludeExpired ends an experiment that reached max_duration. The winner is
// the recommended arm if the result is significant, otherwise control.
func (t *Ticker) concludeExpired(ctx context.Context, exp *Experiment) {
	winner := exp.ControlVersionID
	note := "max_duration reached without a significant result; keeping control"
	if result, err := t.service.EvaluateExperiment(ctx, exp.ID); err == nil && result.Final && result.IsSignificant {
		note = "max_duration reached: " + result.Recommendation
		if result.Recommendation == "ADOPT_TREATMENT" {
			winner = exp.TreatmentVersionID
		}
	}
	if _, err := t.service.Conclude(ctx, exp.ID, winner, note); err != nil {
		if t.logger != nil {
			t.logger.Warn("experiment ticker: concluding expired experiment failed", "experiment_id", exp.ID, "error", err)
		}
		return
	}
	if t.logger != nil {
		t.logger.Info("experiment concluded at max_duration", "experiment_id", exp.ID, "winner", winner)
	}
}

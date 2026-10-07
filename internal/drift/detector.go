package drift

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// DriftResult holds the output of a drift check.
type DriftResult struct {
	ID             uuid.UUID `json:"id"`
	ModelVersionID uuid.UUID `json:"model_version_id"`
	Metric         string    `json:"metric"`
	PSIValue       float64   `json:"psi_value"`
	Severity       string    `json:"severity"`
	BaselineFrom   time.Time `json:"baseline_from"`
	BaselineTo     time.Time `json:"baseline_to"`
	CurrentFrom    time.Time `json:"current_from"`
	CurrentTo      time.Time `json:"current_to"`
	BaselineSample int       `json:"baseline_sample"`
	CurrentSample  int       `json:"current_sample"`
	Message        string    `json:"message"`
	CheckedAt      time.Time `json:"checked_at"`
}

// TelemetryRepository loads raw telemetry values.
type TelemetryRepository interface {
	GetLatencyValues(ctx context.Context, modelVersionID uuid.UUID, from, to time.Time, limit int) ([]float64, error)
	GetConfidenceValues(ctx context.Context, modelVersionID uuid.UUID, from, to time.Time, limit int) ([]float64, error)
}

// ResultStore persists drift results.
type ResultStore interface {
	SaveDriftResult(ctx context.Context, result *DriftResult) error
	GetLatestResults(ctx context.Context, modelVersionID uuid.UUID) ([]DriftResult, error)
	GetHistory(ctx context.Context, modelVersionID uuid.UUID, limit int) ([]DriftResult, error)
}

// Detector checks for model drift.
type Detector struct {
	repo   TelemetryRepository
	store  ResultStore
	logger *slog.Logger
}

// NewDetector creates a new Detector.
func NewDetector(repo TelemetryRepository, store ResultStore, logger *slog.Logger) *Detector {
	return &Detector{repo: repo, store: store, logger: logger}
}

const minSamples = 50

// CheckDrift compares recent telemetry against a baseline period.
func (d *Detector) CheckDrift(ctx context.Context, modelVersionID uuid.UUID, baselineWindow, currentWindow time.Duration) ([]DriftResult, error) {
	now := time.Now()
	baselineFrom := now.Add(-baselineWindow - currentWindow)
	baselineTo := now.Add(-currentWindow)
	currentFrom := now.Add(-currentWindow)
	currentTo := now

	var results []DriftResult

	// Check latency drift
	baselineLat, err := d.repo.GetLatencyValues(ctx, modelVersionID, baselineFrom, baselineTo, 10000)
	if err != nil {
		return nil, fmt.Errorf("loading baseline latencies: %w", err)
	}
	currentLat, err := d.repo.GetLatencyValues(ctx, modelVersionID, currentFrom, currentTo, 10000)
	if err != nil {
		return nil, fmt.Errorf("loading current latencies: %w", err)
	}

	if len(baselineLat) >= minSamples && len(currentLat) >= minSamples {
		psi := PSIFromRawValues(baselineLat, currentLat, 10)
		sev := classifySeverity(psi)
		r := DriftResult{
			ModelVersionID: modelVersionID, Metric: "latency", PSIValue: psi, Severity: sev,
			BaselineFrom: baselineFrom, BaselineTo: baselineTo, CurrentFrom: currentFrom, CurrentTo: currentTo,
			BaselineSample: len(baselineLat), CurrentSample: len(currentLat),
			Message: fmt.Sprintf("Latency PSI=%.4f (%s)", psi, sev), CheckedAt: now,
		}
		results = append(results, r)
		if d.store != nil {
			_ = d.store.SaveDriftResult(ctx, &r)
		}
	}

	// Check confidence drift
	baselineConf, err := d.repo.GetConfidenceValues(ctx, modelVersionID, baselineFrom, baselineTo, 10000)
	if err != nil {
		return nil, fmt.Errorf("loading baseline confidence: %w", err)
	}
	currentConf, err := d.repo.GetConfidenceValues(ctx, modelVersionID, currentFrom, currentTo, 10000)
	if err != nil {
		return nil, fmt.Errorf("loading current confidence: %w", err)
	}

	if len(baselineConf) >= minSamples && len(currentConf) >= minSamples {
		psi := PSIFromRawValues(baselineConf, currentConf, 10)
		sev := classifySeverity(psi)
		r := DriftResult{
			ModelVersionID: modelVersionID, Metric: "confidence", PSIValue: psi, Severity: sev,
			BaselineFrom: baselineFrom, BaselineTo: baselineTo, CurrentFrom: currentFrom, CurrentTo: currentTo,
			BaselineSample: len(baselineConf), CurrentSample: len(currentConf),
			Message: fmt.Sprintf("Confidence PSI=%.4f (%s)", psi, sev), CheckedAt: now,
		}
		results = append(results, r)
		if d.store != nil {
			_ = d.store.SaveDriftResult(ctx, &r)
		}
	}

	return results, nil
}

func classifySeverity(psi float64) string {
	switch {
	case psi >= 0.25:
		return "CRITICAL"
	case psi >= 0.1:
		return "WARNING"
	default:
		return "NONE"
	}
}

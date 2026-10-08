package drift

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/alert"
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
	// LatestResult returns the newest result for a version and metric, or nil.
	LatestResult(ctx context.Context, modelVersionID uuid.UUID, metric string) (*DriftResult, error)
	// RefreshDriftResult overwrites a stored result's values and checked_at.
	RefreshDriftResult(ctx context.Context, id uuid.UUID, result *DriftResult) error
}

// FeatureRepository loads feature specs and histograms (implemented by *Adapter).
type FeatureRepository interface {
	FeatureSpecsForVersion(ctx context.Context, versionID uuid.UUID) ([]FeatureSpec, error)
	FeatureCounts(ctx context.Context, versionID uuid.UUID, feature string, bins int, from, to time.Time) ([]float64, error)
}

// PointStore records every check's PSI (implemented by *Adapter).
type PointStore interface {
	AddPoint(ctx context.Context, r *DriftResult) error
}

// AlertSender delivers alerts (implemented by *alert.Notifier).
type AlertSender interface {
	Send(a alert.Alert)
}

// Detector checks for model drift.
type Detector struct {
	repo     TelemetryRepository
	store    ResultStore
	features FeatureRepository
	alerts   AlertSender
	logger   *slog.Logger
}

// SetFeatureRepository enables feature-level drift.
func (d *Detector) SetFeatureRepository(f FeatureRepository) {
	d.features = f
}

// SetAlertSender makes transitions into CRITICAL send a drift alert.
func (d *Detector) SetAlertSender(a AlertSender) {
	d.alerts = a
}

// record stores a result without repeating unchanged states: if the newest
// stored result for the same version and metric has the same severity it is
// refreshed in place (new PSI, samples, checked_at); a new row is written
// only when the severity changes. A change into CRITICAL sends one alert.
func (d *Detector) record(ctx context.Context, r *DriftResult) {
	if d.store == nil {
		return
	}
	if ps, ok := d.store.(PointStore); ok {
		if err := ps.AddPoint(ctx, r); err != nil && d.logger != nil {
			d.logger.Warn("drift: recording the PSI point failed", "error", err)
		}
	}
	latest, err := d.store.LatestResult(ctx, r.ModelVersionID, r.Metric)
	if err != nil {
		if d.logger != nil {
			d.logger.Warn("drift: loading latest result failed", "version_id", r.ModelVersionID, "error", err)
		}
		return
	}
	if latest != nil && latest.Severity == r.Severity {
		r.ID = latest.ID
		if err := d.store.RefreshDriftResult(ctx, latest.ID, r); err != nil && d.logger != nil {
			d.logger.Warn("drift: refreshing result failed", "error", err)
		}
		return
	}
	if err := d.store.SaveDriftResult(ctx, r); err != nil {
		if d.logger != nil {
			d.logger.Warn("drift: saving result failed", "error", err)
		}
		return
	}
	if r.Severity == "CRITICAL" && d.alerts != nil {
		previous := "NONE"
		if latest != nil {
			previous = latest.Severity
		}
		d.alerts.Send(alert.Alert{Type: alert.TypeDrift, Drift: &alert.Drift{
			ModelVersionID: r.ModelVersionID.String(), Metric: r.Metric, PSI: r.PSIValue,
			Severity: r.Severity, PreviousSeverity: previous,
			BaselineSample: r.BaselineSample, CurrentSample: r.CurrentSample, Message: r.Message,
		}})
	}
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
		d.record(ctx, &r)
		results = append(results, r)
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
		d.record(ctx, &r)
		results = append(results, r)
	}

	featureResults, err := d.checkFeatures(ctx, modelVersionID, baselineFrom, baselineTo, currentFrom, currentTo, now)
	if err != nil {
		return nil, err
	}
	return append(results, featureResults...), nil
}

// checkFeatures compares each configured feature's histogram of the current
// window with the baseline's (PSI over the configured bins).
func (d *Detector) checkFeatures(ctx context.Context, versionID uuid.UUID, baselineFrom, baselineTo, currentFrom, currentTo, now time.Time) ([]DriftResult, error) {
	if d.features == nil {
		return nil, nil
	}
	specs, err := d.features.FeatureSpecsForVersion(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("loading feature specs: %w", err)
	}
	var results []DriftResult
	for _, f := range specs {
		bins := len(f.Edges) + 1
		base, err := d.features.FeatureCounts(ctx, versionID, f.Name, bins, baselineFrom, baselineTo)
		if err != nil {
			return nil, err
		}
		cur, err := d.features.FeatureCounts(ctx, versionID, f.Name, bins, currentFrom, currentTo)
		if err != nil {
			return nil, err
		}
		nBase, nCur := sum(base), sum(cur)
		if nBase < minSamples || nCur < minSamples {
			continue
		}
		psi := PSI(normalize(base, nBase), normalize(cur, nCur))
		sev := classifySeverity(psi)
		r := DriftResult{
			ModelVersionID: versionID, Metric: FeatureMetricPrefix + f.Name, PSIValue: psi, Severity: sev,
			BaselineFrom: baselineFrom, BaselineTo: baselineTo, CurrentFrom: currentFrom, CurrentTo: currentTo,
			BaselineSample: int(nBase), CurrentSample: int(nCur),
			Message: fmt.Sprintf("Feature %s PSI=%.4f (%s)", f.Name, psi, sev), CheckedAt: now,
		}
		d.record(ctx, &r)
		results = append(results, r)
	}
	return results, nil
}

func sum(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x
	}
	return s
}

func normalize(v []float64, total float64) []float64 {
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = x / total
	}
	return out
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

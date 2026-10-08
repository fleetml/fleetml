// Package alert delivers rollout and drift alerts. Every alert is published on
// NATS (fleetml.alerts.<type>) and delivered to the project's alert sinks
// (webhook, Slack, email) and to the server-wide ALERT_WEBHOOK_URL, each with
// up to five attempts and a delivery log.
package alert

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/metrics"
)

// Alert types; the NATS subject is SubjectPrefix + type.
const (
	TypeRollback  = "rollback"
	TypeDrift     = "drift"
	TypeTest      = "test"
	SubjectPrefix = "fleetml.alerts."
)

// Webhook headers.
const (
	HeaderEvent     = "X-FleetML-Event"     // alert type
	HeaderDelivery  = "X-FleetML-Delivery"  // alert ID, stable across retries
	HeaderSignature = "X-FleetML-Signature" // "sha256=" + hex HMAC-SHA256(secret, body)
)

// Alert is the JSON payload sent on NATS and to webhooks.
type Alert struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
	ProjectID  string    `json:"project_id,omitempty"`
	Model      *ModelRef `json:"model,omitempty"`
	Rollback   *Rollback `json:"rollback,omitempty"`
	Drift      *Drift    `json:"drift,omitempty"`
}

// ModelRef names the model version an alert is about.
type ModelRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Rollback describes a rolled-back deployment.
type Rollback struct {
	DeploymentID      string `json:"deployment_id"`
	ProjectID         string `json:"project_id"`
	ModelVersionID    string `json:"model_version_id"`
	PreviousVersionID string `json:"previous_version_id,omitempty"`
	Strategy          string `json:"strategy"`
	Stage             int    `json:"stage"`
	// Trigger is "health_gate" for automatic rollbacks, "manual" otherwise.
	Trigger string `json:"trigger"`
	Reason  string `json:"reason"`
}

// Drift describes a drift result that became CRITICAL.
type Drift struct {
	ModelVersionID   string  `json:"model_version_id"`
	Metric           string  `json:"metric"`
	PSI              float64 `json:"psi"`
	Severity         string  `json:"severity"`
	PreviousSeverity string  `json:"previous_severity"`
	BaselineSample   int     `json:"baseline_sample"`
	CurrentSample    int     `json:"current_sample"`
	Message          string  `json:"message"`
}

// Publisher publishes raw bytes on a subject (satisfied by *nats.Conn).
type Publisher interface {
	Publish(subject string, data []byte) error
}

// VersionLookup finds the project, model name and version of a model version.
type VersionLookup interface {
	DescribeVersion(ctx context.Context, versionID string) (projectID, modelName, version string, err error)
}

// SinkStore lists a project's enabled sinks and records delivery attempts.
type SinkStore interface {
	EnabledSinks(ctx context.Context, projectID string) ([]SinkConfig, error)
	RecordDelivery(ctx context.Context, d Delivery) error
}

// Notifier sends alerts.
type Notifier struct {
	nats      Publisher
	global    *SinkConfig // ALERT_WEBHOOK_URL, for every project
	store     SinkStore
	lookup    VersionLookup
	env       SinkEnv
	backoff   []time.Duration // waits before each retry: len+1 attempts
	logger    *slog.Logger
	wg        sync.WaitGroup
	deliverer func(ctx context.Context, s SinkConfig, a Alert, body []byte) (int, error)
}

// NewNotifier creates a Notifier. webhookURL may be empty; nats may be nil.
func NewNotifier(nats Publisher, webhookURL, secret string, logger *slog.Logger) *Notifier {
	n := &Notifier{
		nats:    nats,
		backoff: []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second},
		logger:  logger,
	}
	if webhookURL != "" {
		n.global = &SinkConfig{Name: "ALERT_WEBHOOK_URL", Kind: KindWebhook, Enabled: true,
			Config: map[string]string{"url": webhookURL, "secret": secret}}
	}
	n.deliverer = n.deliverOnce
	return n
}

// SetStore enables per-project sinks and the delivery log.
func (n *Notifier) SetStore(s SinkStore) { n.store = s }

// SetLookup lets alerts name their project, model and version.
func (n *Notifier) SetLookup(l VersionLookup) { n.lookup = l }

// SetEnv sets the dashboard URL and SMTP settings used by the sinks.
func (n *Notifier) SetEnv(e SinkEnv) { n.env = e }

// Sign returns the signature header value for a body.
func Sign(secret, body []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// Send publishes the alert on NATS and delivers it to the sinks in the
// background. It fills in ID, OccurredAt, ProjectID and Model if it can.
func (n *Notifier) Send(a Alert) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n.complete(ctx, &a)
	body, err := json.Marshal(a)
	if err != nil {
		n.logger.Error("alert: marshalling failed", "error", err)
		return
	}
	if n.nats != nil {
		err := n.nats.Publish(SubjectPrefix+a.Type, body)
		metrics.AlertsSent.WithLabelValues(a.Type, "nats", metrics.Result(err)).Inc()
		if err != nil {
			n.logger.Warn("alert: NATS publish failed", "type", a.Type, "id", a.ID, "error", err)
		}
	}
	var sinks []SinkConfig
	if n.global != nil {
		sinks = append(sinks, *n.global)
	}
	if n.store != nil && a.ProjectID != "" {
		project, err := n.store.EnabledSinks(ctx, a.ProjectID)
		if err != nil {
			n.logger.Warn("alert: loading sinks failed", "project_id", a.ProjectID, "error", err)
		}
		sinks = append(sinks, project...)
	}
	for _, s := range sinks {
		n.wg.Add(1)
		go func(s SinkConfig) {
			defer n.wg.Done()
			n.deliver(s, a, body)
		}(s)
	}
}

func (n *Notifier) complete(ctx context.Context, a *Alert) {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.OccurredAt.IsZero() {
		a.OccurredAt = time.Now().UTC()
	}
	versionID := ""
	switch {
	case a.Rollback != nil:
		versionID = a.Rollback.ModelVersionID
		if a.ProjectID == "" {
			a.ProjectID = a.Rollback.ProjectID
		}
	case a.Drift != nil:
		versionID = a.Drift.ModelVersionID
	}
	if n.lookup != nil && versionID != "" && (a.ProjectID == "" || a.Model == nil) {
		project, name, version, err := n.lookup.DescribeVersion(ctx, versionID)
		if err != nil {
			n.logger.Warn("alert: describing the model version failed", "version_id", versionID, "error", err)
			return
		}
		if a.ProjectID == "" {
			a.ProjectID = project
		}
		if a.Model == nil {
			a.Model = &ModelRef{Name: name, Version: version}
		}
	}
}

// Wait blocks until in-flight deliveries finish (used on shutdown and in tests).
func (n *Notifier) Wait() {
	n.wg.Wait()
}

// deliver tries a sink up to len(backoff)+1 times, recording every attempt;
// the last failure is recorded as a dead letter.
func (n *Notifier) deliver(s SinkConfig, a Alert, body []byte) {
	attempts := len(n.backoff) + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			time.Sleep(n.backoff[attempt-2])
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		code, err := n.deliverer(ctx, s, a, body)
		cancel()
		status := StatusOK
		switch {
		case err != nil && attempt == attempts:
			status = StatusDead
		case err != nil:
			status = StatusError
		}
		n.record(s, a, attempt, status, code, err)
		if err == nil {
			metrics.AlertsSent.WithLabelValues(a.Type, s.Kind, "ok").Inc()
			n.logger.Info("alert delivered", "type", a.Type, "id", a.ID, "sink", s.Name, "attempt", attempt)
			return
		}
	}
	metrics.AlertsSent.WithLabelValues(a.Type, s.Kind, "error").Inc()
	n.logger.Error("alert: delivery failed, dead letter recorded", "type", a.Type, "id", a.ID, "sink", s.Name, "attempts", attempts)
}

func (n *Notifier) record(s SinkConfig, a Alert, attempt int, status string, code int, err error) {
	if n.store == nil || s.ID == "" {
		return
	}
	d := Delivery{SinkID: s.ID, AlertID: a.ID, AlertType: a.Type, Attempt: attempt, Status: status, HTTPStatus: code}
	if err != nil {
		d.Error = err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if rerr := n.store.RecordDelivery(ctx, d); rerr != nil {
		n.logger.Warn("alert: recording the delivery failed", "sink", s.Name, "error", rerr)
	}
}

// SendTest delivers a test alert to one sink, once, and records it.
func (n *Notifier) SendTest(ctx context.Context, s SinkConfig) Delivery {
	a := Alert{ID: uuid.NewString(), Type: TypeTest, OccurredAt: time.Now().UTC(), ProjectID: s.ProjectID}
	body, _ := json.Marshal(a)
	code, err := n.deliverer(ctx, s, a, body)
	status := StatusOK
	if err != nil {
		status = StatusError
	}
	n.record(s, a, 1, status, code, err)
	d := Delivery{SinkID: s.ID, AlertID: a.ID, AlertType: a.Type, Attempt: 1, Status: status, HTTPStatus: code}
	if err != nil {
		d.Error = err.Error()
	}
	return d
}

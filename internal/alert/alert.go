// Package alert delivers rollout and drift alerts: every alert is published
// on NATS (fleetml.alerts.<type>) and, if ALERT_WEBHOOK_URL is set, POSTed
// as JSON with an HMAC-SHA256 signature and up to three retries.
package alert

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Alert types; the NATS subject is SubjectPrefix + type.
const (
	TypeRollback  = "rollback"
	TypeDrift     = "drift"
	SubjectPrefix = "fleetml.alerts."
)

// Webhook headers.
const (
	HeaderEvent     = "X-FleetML-Event"     // alert type
	HeaderDelivery  = "X-FleetML-Delivery"  // alert ID, stable across retries
	HeaderSignature = "X-FleetML-Signature" // "sha256=" + hex HMAC-SHA256(secret, body)
)

// Alert is the JSON payload sent on NATS and to the webhook.
type Alert struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
	Rollback   *Rollback `json:"rollback,omitempty"`
	Drift      *Drift    `json:"drift,omitempty"`
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

// Notifier sends alerts.
type Notifier struct {
	nats       Publisher
	webhookURL string
	secret     []byte
	client     *http.Client
	backoff    []time.Duration // waits before each retry
	logger     *slog.Logger
	wg         sync.WaitGroup
}

// NewNotifier creates a Notifier. webhookURL may be empty; nats may be nil.
func NewNotifier(nats Publisher, webhookURL, secret string, logger *slog.Logger) *Notifier {
	return &Notifier{
		nats:       nats,
		webhookURL: webhookURL,
		secret:     []byte(secret),
		client:     &http.Client{Timeout: 10 * time.Second},
		backoff:    []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second},
		logger:     logger,
	}
}

// Sign returns the signature header value for a body.
func Sign(secret, body []byte) string {
	m := hmac.New(sha256.New, secret)
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// Send publishes the alert on NATS and delivers it to the webhook in the
// background. It fills in ID and OccurredAt if they are empty.
func (n *Notifier) Send(a Alert) {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.OccurredAt.IsZero() {
		a.OccurredAt = time.Now().UTC()
	}
	body, err := json.Marshal(a)
	if err != nil {
		n.logger.Error("alert: marshalling failed", "error", err)
		return
	}
	if n.nats != nil {
		if err := n.nats.Publish(SubjectPrefix+a.Type, body); err != nil {
			n.logger.Warn("alert: NATS publish failed", "type", a.Type, "id", a.ID, "error", err)
		}
	}
	if n.webhookURL == "" {
		return
	}
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		n.deliver(a, body)
	}()
}

// Wait blocks until in-flight webhook deliveries finish (used on shutdown and in tests).
func (n *Notifier) Wait() {
	n.wg.Wait()
}

func (n *Notifier) deliver(a Alert, body []byte) {
	var lastErr error
	for attempt := 0; attempt <= len(n.backoff); attempt++ {
		if attempt > 0 {
			time.Sleep(n.backoff[attempt-1])
		}
		if lastErr = n.post(a, body); lastErr == nil {
			n.logger.Info("alert delivered", "type", a.Type, "id", a.ID, "attempt", attempt+1)
			return
		}
	}
	n.logger.Error("alert: webhook delivery failed", "type", a.Type, "id", a.ID,
		"attempts", len(n.backoff)+1, "error", lastErr)
}

func (n *Notifier) post(a Alert, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderEvent, a.Type)
	req.Header.Set(HeaderDelivery, a.ID)
	if len(n.secret) > 0 {
		req.Header.Set(HeaderSignature, Sign(n.secret, body))
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

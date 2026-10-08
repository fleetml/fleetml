package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Sink kinds.
const (
	KindWebhook = "webhook"
	KindSlack   = "slack"
	KindEmail   = "email"
)

// Delivery statuses.
const (
	StatusOK    = "ok"
	StatusError = "error" // a failed attempt that will be retried
	StatusDead  = "dead"  // the last attempt failed: the alert was not delivered
)

// SinkConfig is one destination for a project's alerts.
//
// Config keys by kind: webhook: url, secret (optional); slack: webhook_url;
// email: to (comma-separated addresses).
type SinkConfig struct {
	ID        string            `json:"id"`
	ProjectID string            `json:"project_id"`
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
	Config    map[string]string `json:"config"`
	Enabled   bool              `json:"enabled"`
	CreatedAt time.Time         `json:"created_at"`
}

// Delivery is one attempt to deliver an alert to a sink.
type Delivery struct {
	ID         string    `json:"id"`
	SinkID     string    `json:"sink_id"`
	AlertID    string    `json:"alert_id"`
	AlertType  string    `json:"alert_type"`
	Attempt    int       `json:"attempt"`
	Status     string    `json:"status"`
	HTTPStatus int       `json:"http_status,omitempty"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// SinkEnv is the server configuration the sinks use.
type SinkEnv struct {
	// DashboardURL makes Slack and email alerts link to the dashboard.
	DashboardURL string
	SMTP         SMTPConfig
}

// SMTPConfig is the mail server for email sinks.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// Configured reports whether email sinks can send.
func (c SMTPConfig) Configured() bool { return c.Host != "" && c.From != "" }

// ValidateSink checks a sink's kind and config; smtp tells whether email is available.
func ValidateSink(kind string, cfg map[string]string, smtpOK bool) error {
	switch kind {
	case KindWebhook:
		return validateURL("url", cfg["url"])
	case KindSlack:
		return validateURL("webhook_url", cfg["webhook_url"])
	case KindEmail:
		if !smtpOK {
			return fmt.Errorf("email sinks need SMTP_HOST and SMTP_FROM on the server")
		}
		if len(recipients(cfg["to"])) == 0 {
			return fmt.Errorf("to: at least one address is required")
		}
		for _, r := range recipients(cfg["to"]) {
			if !strings.Contains(r, "@") {
				return fmt.Errorf("to: %q is not an email address", r)
			}
		}
		return nil
	default:
		return fmt.Errorf("kind must be webhook, slack or email")
	}
}

func validateURL(field, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%s must be an http(s) URL", field)
	}
	return nil
}

func recipients(s string) []string {
	var out []string
	for _, r := range strings.Split(s, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// RedactedConfig hides secrets for API responses.
func RedactedConfig(kind string, cfg map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range cfg {
		out[k] = v
	}
	if out["secret"] != "" {
		out["secret"] = "********"
	}
	if u, err := url.Parse(out["webhook_url"]); err == nil && kind == KindSlack && u.Host != "" {
		out["webhook_url"] = u.Scheme + "://" + u.Host + "/…"
	}
	return out
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// deliverOnce sends one attempt; it returns the HTTP status for HTTP sinks.
func (n *Notifier) deliverOnce(ctx context.Context, s SinkConfig, a Alert, body []byte) (int, error) {
	switch s.Kind {
	case KindWebhook:
		headers := map[string]string{HeaderEvent: a.Type, HeaderDelivery: a.ID}
		if secret := s.Config["secret"]; secret != "" {
			headers[HeaderSignature] = Sign([]byte(secret), body)
		}
		return postJSON(ctx, s.Config["url"], body, headers)
	case KindSlack:
		var payload bytes.Buffer
		enc := json.NewEncoder(&payload)
		enc.SetEscapeHTML(false) // keep Slack's <url|text> links readable
		if err := enc.Encode(slackMessage(a, n.env.DashboardURL)); err != nil {
			return 0, err
		}
		return postJSON(ctx, s.Config["webhook_url"], payload.Bytes(), nil)
	case KindEmail:
		return 0, n.sendEmail(s, a)
	default:
		return 0, fmt.Errorf("unknown sink kind %q", s.Kind)
	}
}

func postJSON(ctx context.Context, target string, body []byte, headers map[string]string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// summary is the human-readable title, details and dashboard path of an alert.
func summary(a Alert) (title string, fields [][2]string, path string) {
	model := ""
	if a.Model != nil {
		model = a.Model.Name + " " + a.Model.Version
	}
	switch {
	case a.Rollback != nil:
		r := a.Rollback
		title = "Rollback: " + strings.TrimSpace(model)
		trigger := "manual rollback"
		if r.Trigger == "health_gate" {
			trigger = "health gate"
		}
		fields = [][2]string{{"Reason", r.Reason}, {"Trigger", trigger}, {"Strategy", r.Strategy}, {"Deployment", r.DeploymentID}}
		path = "/deployments"
	case a.Drift != nil:
		d := a.Drift
		title = "Drift CRITICAL: " + strings.TrimSpace(model) + " " + d.Metric
		fields = [][2]string{{"PSI", strconv.FormatFloat(d.PSI, 'f', 3, 64)}, {"Previous severity", d.PreviousSeverity},
			{"Events", fmt.Sprintf("%d baseline, %d current", d.BaselineSample, d.CurrentSample)}}
		path = "/drift?version=" + d.ModelVersionID
	default:
		title = "FleetML test alert"
		fields = [][2]string{{"Result", "This sink receives FleetML alerts."}}
	}
	return title, fields, path
}

// slackMessage is a Block Kit message for an incoming webhook.
func slackMessage(a Alert, dashboard string) map[string]any {
	title, fields, path := summary(a)
	var sectionFields []map[string]string
	for _, f := range fields {
		if f[1] != "" {
			sectionFields = append(sectionFields, map[string]string{"type": "mrkdwn", "text": "*" + f[0] + "*\n" + f[1]})
		}
	}
	blocks := []map[string]any{
		{"type": "header", "text": map[string]string{"type": "plain_text", "text": title}},
		{"type": "section", "fields": sectionFields},
	}
	if dashboard != "" && path != "" {
		blocks = append(blocks, map[string]any{"type": "section", "text": map[string]string{
			"type": "mrkdwn", "text": "<" + strings.TrimRight(dashboard, "/") + path + "|Open in the FleetML dashboard>"}})
	}
	blocks = append(blocks, map[string]any{"type": "context", "elements": []map[string]string{
		{"type": "mrkdwn", "text": "FleetML · " + a.Type + " · " + a.OccurredAt.Format(time.RFC3339)}}})
	return map[string]any{"text": title, "blocks": blocks}
}

func (n *Notifier) sendEmail(s SinkConfig, a Alert) error {
	cfg := n.env.SMTP
	if !cfg.Configured() {
		return fmt.Errorf("SMTP is not configured on the server")
	}
	title, fields, path := summary(a)
	var b strings.Builder
	for _, f := range fields {
		if f[1] != "" {
			fmt.Fprintf(&b, "%s: %s\r\n", f[0], f[1])
		}
	}
	if n.env.DashboardURL != "" && path != "" {
		fmt.Fprintf(&b, "\r\n%s%s\r\n", strings.TrimRight(n.env.DashboardURL, "/"), path)
	}
	to := recipients(s.Config["to"])
	msg := "From: " + cfg.From + "\r\nTo: " + strings.Join(to, ", ") + "\r\nSubject: [FleetML] " + title +
		"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + b.String()
	port := cfg.Port
	if port == 0 {
		port = 587
	}
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	return smtp.SendMail(net.JoinHostPort(cfg.Host, strconv.Itoa(port)), auth, cfg.From, to, []byte(msg))
}

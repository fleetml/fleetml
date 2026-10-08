// Package metrics defines FleetML's Prometheus metrics, served at /metrics.
package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registry holds every FleetML metric plus the Go runtime and process collectors.
var Registry = prometheus.NewRegistry()

var (
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fleetml_http_requests_total", Help: "REST requests by method, route and status code.",
	}, []string{"method", "route", "status"})
	HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "fleetml_http_request_duration_seconds", Help: "REST request latency by method and route.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})
	GRPCRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fleetml_grpc_requests_total", Help: "gRPC requests by method and status code.",
	}, []string{"method", "code"})
	GRPCDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "fleetml_grpc_request_duration_seconds", Help: "gRPC request latency by method.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})
	TickerRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fleetml_ticker_runs_total", Help: "Background loop runs by loop (deploy, experiment, drift).",
	}, []string{"ticker"})
	TickerDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "fleetml_ticker_run_duration_seconds", Help: "Background loop run time by loop.",
		Buckets: prometheus.DefBuckets,
	}, []string{"ticker"})
	TelemetryAccepted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "fleetml_telemetry_events_accepted_total", Help: "Telemetry events accepted from devices.",
	})
	TelemetryRejected = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "fleetml_telemetry_events_rejected_total",
		Help: "Telemetry events rejected at ingest (invalid, or for a model version outside the device's project).",
	})
	AlertsSent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fleetml_alerts_sent_total", Help: "Alerts by type (rollback, drift), channel (nats, webhook, slack, email) and result (ok, error).",
	}, []string{"type", "channel", "result"})
)

func init() {
	Registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		HTTPRequests, HTTPDuration, GRPCRequests, GRPCDuration, TickerRuns, TickerDuration,
		TelemetryAccepted, TelemetryRejected, AlertsSent)
}

// ObserveTick records one run of a background loop; call it with defer at the start of the run.
func ObserveTick(ticker string, start time.Time) {
	TickerRuns.WithLabelValues(ticker).Inc()
	TickerDuration.WithLabelValues(ticker).Observe(time.Since(start).Seconds())
}

// Result is "ok" for a nil error and "error" otherwise.
func Result(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

// RegisterTelemetryConsumer exposes the telemetry consumer's counters and lag.
func RegisterTelemetryConsumer(dropped, redelivered, duplicates, pending func() int64) {
	f := func(fn func() int64) func() float64 { return func() float64 { return float64(fn()) } }
	Registry.MustRegister(
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "fleetml_telemetry_events_dropped_total", Help: "Accepted telemetry events the consumer could not store.",
		}, f(dropped)),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "fleetml_telemetry_redelivered_total", Help: "Telemetry stream messages delivered to the consumer more than once (after a crash or a failed write).",
		}, f(redelivered)),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "fleetml_telemetry_duplicates_total", Help: "Telemetry events skipped because an event with the same ID was already stored.",
		}, f(duplicates)),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "fleetml_telemetry_consumer_pending", Help: "Telemetry stream messages not yet stored: undelivered plus delivered and not yet acknowledged (the consumer's lag).",
		}, f(pending)),
	)
}

// RegisterDeployments exposes the number of deployments in each status, read at scrape time.
func RegisterDeployments(pool *pgxpool.Pool, logger *slog.Logger) {
	Registry.MustRegister(&deploymentCollector{pool: pool, logger: logger})
}

var deploymentsDesc = prometheus.NewDesc("fleetml_deployments", "Deployments by status.", []string{"status"}, nil)

type deploymentCollector struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func (c *deploymentCollector) Describe(ch chan<- *prometheus.Desc) { ch <- deploymentsDesc }

func (c *deploymentCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rows, err := c.pool.Query(ctx, `SELECT status, COUNT(*) FROM deployments GROUP BY status`)
	if err != nil {
		c.logger.Warn("metrics: counting deployments failed", "error", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int64
		if rows.Scan(&status, &n) == nil {
			ch <- prometheus.MustNewConstMetric(deploymentsDesc, prometheus.GaugeValue, float64(n), status)
		}
	}
}

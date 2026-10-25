// Command fleetml-agent keeps a Linux machine's models in line with what a
// FleetML server assigns. See docs/LINUX_AGENT.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/fleetml/fleetml/internal/agent"
)

type labelFlags map[string]string

func (l labelFlags) String() string { return fmt.Sprint(map[string]string(l)) }
func (l labelFlags) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || k == "" {
		return fmt.Errorf("expected key=value, got %q", v)
	}
	l[k] = val
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fleetml-agent:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "", "YAML config file (e.g. /etc/fleetml-agent/config.yaml)")
	server := flag.String("server", "", "FleetML gRPC address host:port (overrides the config)")
	key := flag.String("enrollment-key", "", "project enrollment key (overrides the config); FLEETML_ENROLLMENT_KEY in the environment\nis used when neither this flag nor the config sets one, and keeps the key out of the process list")
	stateDir := flag.String("state-dir", "", "state directory (overrides the config)")
	modelsDir := flag.String("models-dir", "", "models directory (overrides the config)")
	validator := flag.String("validator", "", "validator command; {file} is the model path (overrides the config)")
	hook := flag.String("post-activate", "", "command run after a model version becomes active (overrides the config)")
	listen := flag.String("telemetry-listen", "", "local telemetry endpoint, e.g. 127.0.0.1:9601 (overrides the config)")
	downloadHost := flag.String("download-host-override", "", "host:port that every model download goes to instead of the server's S3_PUBLIC_ENDPOINT (overrides the config)")
	labels := labelFlags{}
	flag.Var(labels, "label", "device label key=value (repeatable; added to the config's labels)")
	retryFailed := flag.Bool("retry-failed", false, "retry the versions that failed validation now instead of after the hour's wait")
	version := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *version {
		fmt.Println("fleetml-agent", agent.AgentVersion)
		return nil
	}

	cfg, err := agent.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	for flagValue, field := range map[*string]*string{server: &cfg.Server, key: &cfg.EnrollmentKey, stateDir: &cfg.StateDir,
		modelsDir: &cfg.ModelsDir, validator: &cfg.Validator, hook: &cfg.PostActivate, listen: &cfg.TelemetryListen,
		downloadHost: &cfg.DownloadHostOverride} {
		if *flagValue != "" {
			*field = *flagValue
		}
	}
	if cfg.Labels == nil {
		cfg.Labels = map[string]string{}
	}
	for k, v := range labels {
		cfg.Labels[k] = v
	}
	cfg.UseEnvEnrollmentKey(os.Getenv("FLEETML_ENROLLMENT_KEY"))

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	a, err := agent.New(cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = a.Close() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Info("fleetml-agent starting", "version", agent.AgentVersion, "server", cfg.Server, "models_dir", cfg.ModelsDir)
	if *retryFailed {
		n, err := a.RetryFailed()
		if err != nil {
			return fmt.Errorf("clearing the failed versions: %w", err)
		}
		log.Info("cleared the failed versions; they are retried on the next heartbeat", "count", n)
	}
	return a.Run(ctx)
}

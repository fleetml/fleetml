package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	grpclib "google.golang.org/grpc"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	"github.com/fleetml/fleetml/internal/alert"
	grpcapi "github.com/fleetml/fleetml/internal/api/grpc"
	"github.com/fleetml/fleetml/internal/api/rest"
	"github.com/fleetml/fleetml/internal/audit"
	"github.com/fleetml/fleetml/internal/auth"
	"github.com/fleetml/fleetml/internal/config"
	"github.com/fleetml/fleetml/internal/db"
	"github.com/fleetml/fleetml/internal/deltabuild"
	"github.com/fleetml/fleetml/internal/deploy"
	"github.com/fleetml/fleetml/internal/device"
	"github.com/fleetml/fleetml/internal/drift"
	"github.com/fleetml/fleetml/internal/experiment"
	"github.com/fleetml/fleetml/internal/health"
	"github.com/fleetml/fleetml/internal/intelligence"
	"github.com/fleetml/fleetml/internal/metrics"
	"github.com/fleetml/fleetml/internal/registry"
	"github.com/fleetml/fleetml/internal/signing"
	"github.com/fleetml/fleetml/internal/storage"
	"github.com/fleetml/fleetml/internal/telemetry"
)

// dependencyTimeout is how long startup waits for Postgres, S3 and NATS, so the
// server can start alongside them (docker compose up, a cold dev stack).
const dependencyTimeout = 60 * time.Second

// waitFor calls fn until it succeeds, retrying every second until timeout.
func waitFor(ctx context.Context, what string, timeout time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	for attempt := 1; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		if attempt == 1 {
			slog.Info("waiting for dependency", "dependency", what, "error", err)
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Second):
		}
	}
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	setupLogger(cfg.LogLevel, cfg.LogFormat)
	slog.Info("starting FleetML server")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if cfg.MigrateOnStart {
		if err := waitFor(ctx, "database migrations", dependencyTimeout, func() error { return db.Migrate(cfg.DatabaseURL) }); err != nil {
			return fmt.Errorf("running migrations: %w", err)
		}
	}

	var pool *pgxpool.Pool
	err = waitFor(ctx, "database", dependencyTimeout, func() (err error) {
		pool, err = db.NewPool(ctx, cfg.DatabaseURL)
		return err
	})
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	s3Client, err := storage.NewS3Client(storage.S3Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		UseSSL:    cfg.S3UseSSL,
		Bucket:    cfg.S3Bucket,
	})
	if err != nil {
		return fmt.Errorf("creating s3 client: %w", err)
	}
	if err := waitFor(ctx, "s3", dependencyTimeout, func() error { return s3Client.EnsureBucket(ctx) }); err != nil {
		return fmt.Errorf("preparing s3 bucket: %w", err)
	}
	// Upload and download URLs are signed for the address clients use.
	if cfg.S3PublicEndpoint != "" {
		signer, err := storage.NewPublicSigner(storage.S3Config{
			AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, UseSSL: cfg.S3UseSSL, Bucket: cfg.S3Bucket,
		}, cfg.S3PublicEndpoint)
		if err != nil {
			return fmt.Errorf("creating s3 public signer: %w", err)
		}
		s3Client.SetPublicSigner(signer)
		slog.Info("presigned URLs use S3_PUBLIC_ENDPOINT", "endpoint", cfg.S3PublicEndpoint)
	}

	var nc *nats.Conn
	err = waitFor(ctx, "nats", dependencyTimeout, func() (err error) {
		nc, err = nats.Connect(cfg.NatsURL)
		return err
	})
	if err != nil {
		return fmt.Errorf("connecting to nats: %w", err)
	}
	defer nc.Close()
	slog.Info("nats connection established")

	logger := slog.Default()

	readiness := health.NewChecker(logger)
	readiness.Add("db", pool.Ping)
	readiness.Add("s3", s3Client.HealthCheck)
	readiness.Add("nats", func(context.Context) error {
		if !nc.IsConnected() {
			return fmt.Errorf("nats connection is %s", nc.Status())
		}
		return nil
	})

	// Registry repositories + service
	projectRepo := registry.NewProjectRepo(pool)
	modelRepo := registry.NewModelRepo(pool)
	versionRepo := registry.NewVersionRepo(pool)
	variantRepo := registry.NewVariantRepo(pool)
	auditLogger := audit.NewLogger(pool)
	registrySvc := registry.NewService(projectRepo, modelRepo, versionRepo, variantRepo, s3Client, auditLogger, logger)
	keyring, err := signing.NewKeyring(pool, cfg.SigningMasterKey)
	if err != nil {
		return err
	}
	registrySvc.SetSigner(keyring)
	deltas := deltabuild.New(pool, s3Client, int64(cfg.DeltaMaxSizeMB)<<20, logger)
	registrySvc.SetReadyHook(deltas.BuildFor)
	if n, err := keyring.Backfill(ctx); err != nil {
		return fmt.Errorf("signing published variants: %w", err)
	} else if n > 0 {
		slog.Info("signed variants published before artifact signing", "count", n)
	}

	// Device repositories + service
	deviceRepo := device.NewDeviceRepo(pool)
	deviceClassRepo := device.NewDeviceClassRepo(pool)
	deviceSvc := device.NewService(deviceRepo, deviceClassRepo, cfg.JWTSecret, logger)
	deviceSvc.SetTokenTTL(cfg.DeviceTokenTTL)
	deviceSvc.SetConfig(device.Config{
		HeartbeatSecs:           uint32(cfg.DeviceHeartbeatInterval.Seconds()),
		BackgroundHeartbeatSecs: uint32(cfg.DeviceBackgroundHeartbeatInterval.Seconds()),
		TelemetryBatchSize:      uint32(cfg.DeviceTelemetryBatchSize),
		TelemetryFlushSecs:      uint32(cfg.DeviceTelemetryFlushInterval.Seconds()),
		MaxCacheMB:              device.DefaultConfig().MaxCacheMB,
		DownloadPolicy:          cfg.DeviceDownloadPolicy,
	})
	enrollmentSvc := auth.NewEnrollmentService(auth.NewEnrollmentKeyRepo(pool))
	deviceSvc.SetEnrollmentVerifier(enrollmentSvc)

	// Deploy service + engine
	deployRepo := deploy.NewRepo(pool)
	deploySvc := deploy.NewService(deployRepo, logger)
	telemetryAdapter := deploy.NewTelemetryAdapter(pool)
	deviceCountAdapter := deploy.NewDeviceCountAdapter(pool)
	deployEngine := deploy.NewEngine(deployRepo, telemetryAdapter, deviceCountAdapter, logger)
	deploySvc.SetEngine(deployEngine)
	alerts := alert.NewNotifier(nc, cfg.AlertWebhookURL, cfg.AlertWebhookSecret, logger)
	if cfg.AlertWebhookURL != "" && cfg.AlertWebhookSecret == "" {
		slog.Warn("ALERT_WEBHOOK_URL is set without ALERT_WEBHOOK_SECRET: webhook bodies will not be signed")
	}
	alertRepo := alert.NewRepo(pool)
	alerts.SetStore(alertRepo)
	alerts.SetLookup(alertRepo)
	smtpCfg := alert.SMTPConfig{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.SMTPFrom}
	alerts.SetEnv(alert.SinkEnv{DashboardURL: cfg.DashboardURL, SMTP: smtpCfg})
	deployEngine.SetAlertSender(alerts)
	versionChecker := deploy.NewVersionAdapter(pool)
	deploySvc.SetVersionChecker(versionChecker)
	deployTicker := deploy.NewTicker(deployEngine, deployRepo, cfg.DeployTickInterval, logger)
	go deployTicker.Start(ctx)

	// Intelligence layer
	profiler := intelligence.NewProfiler(logger)
	selector := intelligence.NewSelector(logger)
	intelVariantRepo := intelligence.NewVariantRepoAdapter(pool)
	orchestrator := intelligence.NewOrchestrator(profiler, selector, intelVariantRepo, s3Client, logger)

	// Wire intelligence into device service
	intelDeployRepo := intelligence.NewDeploymentRepoAdapter(pool)
	bridge := intelligence.NewBridge(orchestrator, intelDeployRepo)
	deviceSvc.SetAssignmentComputer(bridge)

	// Telemetry
	telemetryRepo := telemetry.NewRepository(pool)
	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("JetStream: %w", err)
	}
	if _, err := telemetry.EnsureStream(ctx, js); err != nil {
		return fmt.Errorf("creating the telemetry stream (NATS must run with JetStream, nats-server -js): %w", err)
	}
	telemetryPub := telemetry.NewPublisher(js, logger)
	telemetrySvc := telemetry.NewService(telemetryRepo, telemetryPub, logger)
	telemetryConsumer := telemetry.NewConsumer(js, telemetryRepo, logger)
	metrics.RegisterTelemetryConsumer(telemetryConsumer.Dropped, telemetryConsumer.Redelivered, telemetryConsumer.Duplicates, telemetryConsumer.Pending)
	metrics.RegisterDeployments(pool, logger)
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		if err := telemetryConsumer.Start(ctx); err != nil {
			slog.Error("telemetry consumer error", "error", err)
		}
	}()

	// gRPC server with auth interceptor
	grpcServer := grpcapi.NewServer(cfg.GRPCReflection,
		grpclib.ChainUnaryInterceptor(metrics.UnaryServerInterceptor(), grpcapi.AuthInterceptor(deviceSvc)),
	)
	go grpcServer.WatchReadiness(ctx, func(ctx context.Context) bool {
		_, ok := readiness.Check(ctx)
		return ok
	}, 10*time.Second)
	deviceGRPC := grpcapi.NewDeviceServiceServer(deviceSvc, logger)
	deviceGRPC.SetSigning(keyring)
	deviceGRPC.SetDeltas(deltas)
	deviceGRPC.SetFeatures(drift.NewAdapter(pool))
	fleetmlv1.RegisterDeviceServiceServer(grpcServer.GRPCServer(), deviceGRPC)
	telemetryGRPC := grpcapi.NewTelemetryServiceServer(telemetrySvc, logger)
	fleetmlv1.RegisterTelemetryServiceServer(grpcServer.GRPCServer(), telemetryGRPC)

	// REST server
	// Experiment service
	experimentRepo := experiment.NewRepository(pool)
	experimentSvc := experiment.NewService(experimentRepo, logger)
	experimentSvc.SetVersionChecker(versionChecker)
	experimentTicker := experiment.NewTicker(experimentSvc, cfg.ExperimentTickInterval, logger)
	go experimentTicker.Start(ctx)
	experimentBridge := experiment.NewBridge(experimentSvc, orchestrator, logger)
	deviceSvc.SetExperimentAssigner(experimentBridge)

	// Drift detection
	driftAdapter := drift.NewAdapter(pool)
	driftDetector := drift.NewDetector(driftAdapter, driftAdapter, logger)
	driftDetector.SetAlertSender(alerts)
	driftDetector.SetFeatureRepository(driftAdapter)
	driftTicker := drift.NewTicker(driftDetector, driftAdapter, cfg.DriftTickInterval, logger)
	go driftTicker.Start(ctx)

	// Admin API keys: make sure a fresh install has one.
	apiKeySvc := auth.NewAPIKeyService(auth.NewAPIKeyRepo(pool))
	generated, err := apiKeySvc.Bootstrap(ctx, cfg.AdminKey)
	if err != nil {
		return fmt.Errorf("bootstrapping admin api key: %w", err)
	}
	if generated != "" {
		// Shown exactly once; only its hash is stored.
		slog.Warn("no admin API key existed and FLEETML_ADMIN_KEY is not set: generated one, store it now, it will not be shown again",
			"admin_api_key", generated)
	}

	sessionSvc := auth.NewSessionService(auth.NewSessionRepo(pool), apiKeySvc, cfg.JWTSecret, cfg.SessionTTL)
	restServer := rest.NewServer(readiness, registrySvc, deviceSvc, deploySvc, orchestrator, telemetrySvc, experimentSvc, driftDetector, driftAdapter, apiKeySvc, enrollmentSvc, sessionSvc, cfg.CORSOrigins, logger)
	restServer.EnableSigning(rest.NewSigningHandler(keyring, logger))
	restServer.EnableAlertSinks(rest.NewAlertSinkHandler(alertRepo, alerts, smtpCfg.Configured(), logger))

	errCh := make(chan error, 2)

	go func() {
		errCh <- grpcServer.ListenAndServe(ctx, cfg.GRPCAddr())
	}()

	go func() {
		errCh <- restServer.ListenAndServe(ctx, cfg.RESTAddr())
	}()

	select {
	case err := <-errCh:
		if err != nil {
			slog.Error("server error", "error", err)
			cancel()
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	// Let the telemetry consumer drain its buffer before the DB pool closes.
	select {
	case <-consumerDone:
	case <-time.After(15 * time.Second):
		slog.Warn("telemetry consumer did not stop in time")
	}

	// Give in-flight webhook deliveries (with retries) a chance to finish.
	alertsDone := make(chan struct{})
	go func() { alerts.Wait(); close(alertsDone) }()
	select {
	case <-alertsDone:
	case <-time.After(10 * time.Second):
		slog.Warn("alert deliveries still pending at shutdown")
	}

	slog.Info("FleetML server stopped")
	return nil
}

func setupLogger(level, format string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if strings.ToLower(format) == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}

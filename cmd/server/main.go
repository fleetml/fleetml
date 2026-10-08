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

	"github.com/nats-io/nats.go"
	grpclib "google.golang.org/grpc"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	grpcapi "github.com/fleetml/fleetml/internal/api/grpc"
	"github.com/fleetml/fleetml/internal/api/rest"
	"github.com/fleetml/fleetml/internal/audit"
	"github.com/fleetml/fleetml/internal/auth"
	"github.com/fleetml/fleetml/internal/config"
	"github.com/fleetml/fleetml/internal/db"
	"github.com/fleetml/fleetml/internal/deploy"
	"github.com/fleetml/fleetml/internal/device"
	"github.com/fleetml/fleetml/internal/drift"
	"github.com/fleetml/fleetml/internal/experiment"
	"github.com/fleetml/fleetml/internal/intelligence"
	"github.com/fleetml/fleetml/internal/registry"
	"github.com/fleetml/fleetml/internal/storage"
	"github.com/fleetml/fleetml/internal/telemetry"
)

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
		if err := db.Migrate(cfg.DatabaseURL); err != nil {
			return fmt.Errorf("running migrations: %w", err)
		}
	}

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
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
	if err := s3Client.EnsureBucket(ctx); err != nil {
		return fmt.Errorf("preparing s3 bucket: %w", err)
	}

	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		return fmt.Errorf("connecting to nats: %w", err)
	}
	defer nc.Close()
	slog.Info("nats connection established")

	natsOK := func() bool {
		return nc.IsConnected()
	}

	logger := slog.Default()

	// Registry repositories + service
	projectRepo := registry.NewProjectRepo(pool)
	modelRepo := registry.NewModelRepo(pool)
	versionRepo := registry.NewVersionRepo(pool)
	variantRepo := registry.NewVariantRepo(pool)
	auditLogger := audit.NewLogger(pool)
	registrySvc := registry.NewService(projectRepo, modelRepo, versionRepo, variantRepo, s3Client, auditLogger, logger)

	// Device repositories + service
	deviceRepo := device.NewDeviceRepo(pool)
	deviceClassRepo := device.NewDeviceClassRepo(pool)
	deploymentAdapter := device.NewDeploymentAdapter(pool)
	variantAdapter := device.NewVariantAdapter(pool)
	deviceSvc := device.NewService(deviceRepo, deviceClassRepo, deploymentAdapter, variantAdapter, s3Client, cfg.JWTSecret, logger)
	deviceSvc.SetTokenTTL(cfg.DeviceTokenTTL)
	enrollmentSvc := auth.NewEnrollmentService(auth.NewEnrollmentKeyRepo(pool))
	deviceSvc.SetEnrollmentVerifier(enrollmentSvc)

	// Deploy service + engine
	deployRepo := deploy.NewRepo(pool)
	deploySvc := deploy.NewService(deployRepo, logger)
	telemetryAdapter := deploy.NewTelemetryAdapter(pool)
	deviceCountAdapter := deploy.NewDeviceCountAdapter(pool)
	deployEngine := deploy.NewEngine(deployRepo, telemetryAdapter, deviceCountAdapter, logger)
	deploySvc.SetEngine(deployEngine)
	deployTicker := deploy.NewTicker(deployEngine, deployRepo, 30*time.Second, logger)
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
	telemetryPub := telemetry.NewPublisher(nc, logger)
	telemetrySvc := telemetry.NewService(telemetryRepo, telemetryPub, logger)
	telemetryConsumer := telemetry.NewConsumer(nc, telemetryRepo, logger)
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		if err := telemetryConsumer.Start(ctx); err != nil {
			slog.Error("telemetry consumer error", "error", err)
		}
	}()

	// gRPC server with auth interceptor
	grpcServer := grpcapi.NewServer(
		grpclib.UnaryInterceptor(grpcapi.AuthInterceptor(deviceSvc)),
	)
	deviceGRPC := grpcapi.NewDeviceServiceServer(deviceSvc, logger)
	fleetmlv1.RegisterDeviceServiceServer(grpcServer.GRPCServer(), deviceGRPC)
	telemetryGRPC := grpcapi.NewTelemetryServiceServer(telemetrySvc, logger)
	fleetmlv1.RegisterTelemetryServiceServer(grpcServer.GRPCServer(), telemetryGRPC)

	// REST server
	// Experiment service
	experimentRepo := experiment.NewRepository(pool)
	experimentSvc := experiment.NewService(experimentRepo, logger)
	experimentTicker := experiment.NewTicker(experimentSvc, 60*time.Second, logger)
	go experimentTicker.Start(ctx)
	experimentBridge := experiment.NewBridge(experimentSvc, orchestrator, logger)
	deviceSvc.SetExperimentAssigner(experimentBridge)

	// Drift detection
	driftAdapter := drift.NewAdapter(pool)
	driftDetector := drift.NewDetector(driftAdapter, driftAdapter, logger)
	driftTicker := drift.NewTicker(driftDetector, driftAdapter, 5*time.Minute, logger)
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

	restServer := rest.NewServer(pool, s3Client, natsOK, registrySvc, deviceSvc, deploySvc, orchestrator, telemetrySvc, experimentSvc, driftDetector, driftAdapter, apiKeySvc, enrollmentSvc, cfg.CORSOrigins, logger)

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

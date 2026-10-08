package rest

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fleetml/fleetml/internal/auth"
	"github.com/fleetml/fleetml/internal/deploy"
	"github.com/fleetml/fleetml/internal/device"
	"github.com/fleetml/fleetml/internal/drift"
	"github.com/fleetml/fleetml/internal/experiment"
	"github.com/fleetml/fleetml/internal/intelligence"
	"github.com/fleetml/fleetml/internal/registry"
	"github.com/fleetml/fleetml/internal/storage"
	"github.com/fleetml/fleetml/internal/telemetry"
)

// Server is the REST API server.
type Server struct {
	router        *gin.Engine
	pool          *pgxpool.Pool
	s3            *storage.S3Client
	natsOK        func() bool
	service       *registry.Service
	deviceService *device.Service
	deploySvc     *deploy.Service
	orchestrator  *intelligence.Orchestrator
	telemetrySvc  *telemetry.Service
	experimentSvc *experiment.Service
	driftDetector *drift.Detector
	driftStore    drift.ResultStore
	apiKeys       *auth.APIKeyService
	corsOrigins   string
	logger        *slog.Logger
}

// NewServer creates a new REST server.
func NewServer(pool *pgxpool.Pool, s3 *storage.S3Client, natsOK func() bool, service *registry.Service, deviceSvc *device.Service, deploySvc *deploy.Service, orchestrator *intelligence.Orchestrator, telemetrySvc *telemetry.Service, experimentSvc *experiment.Service, driftDetector *drift.Detector, driftStore drift.ResultStore, apiKeys *auth.APIKeyService, corsOrigins string, logger *slog.Logger) *Server {
	if apiKeys == nil {
		panic("rest.NewServer: an API key service is required")
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	s := &Server{
		router:        r,
		pool:          pool,
		s3:            s3,
		natsOK:        natsOK,
		service:       service,
		deviceService: deviceSvc,
		deploySvc:     deploySvc,
		orchestrator:  orchestrator,
		telemetrySvc:  telemetrySvc,
		experimentSvc: experimentSvc,
		driftDetector: driftDetector,
		driftStore:    driftStore,
		apiKeys:       apiKeys,
		corsOrigins:   corsOrigins,
		logger:        logger,
	}

	// CORS runs first so preflight requests (which carry no API key) are answered.
	r.Use(CORS(s.corsOrigins))
	r.Use(RequestID())
	r.Use(ErrorRecovery(logger))
	r.Use(RequestLogger(logger))

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.router.GET("/healthz", s.healthCheck)

	// Everything under /api/v1 requires an admin API key; /healthz stays open.
	api := s.router.Group("/api/v1", RequireAPIKey(s.apiKeys, s.logger))

	NewAPIKeyHandler(s.apiKeys, s.logger).RegisterRoutes(api)

	NewProjectHandler(s.service, s.logger).RegisterRoutes(api)
	NewModelHandler(s.service, s.logger).RegisterRoutes(api)
	NewVersionHandler(s.service, s.logger).RegisterRoutes(api)
	NewVariantHandler(s.service, s.logger).RegisterRoutes(api)
	if s.deviceService != nil {
		NewDeviceHandler(s.deviceService, s.logger).RegisterRoutes(api)
	}
	if s.deploySvc != nil {
		NewDeploymentHandler(s.deploySvc, s.logger).RegisterRoutes(api)
	}
	if s.orchestrator != nil && s.deviceService != nil && s.deploySvc != nil {
		NewIntelligenceHandler(s.orchestrator, s.deviceService, s.deploySvc, s.logger).RegisterRoutes(api)
	}
	if s.telemetrySvc != nil {
		NewTelemetryHandler(s.telemetrySvc, s.logger).RegisterRoutes(api)
	}
	if s.experimentSvc != nil {
		NewExperimentHandler(s.experimentSvc, s.logger).RegisterRoutes(api)
	}
	if s.driftDetector != nil && s.driftStore != nil {
		NewDriftHandler(s.driftDetector, s.driftStore, s.logger).RegisterRoutes(api)
	}
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) healthCheck(c *gin.Context) {
	ctx := c.Request.Context()

	dbStatus := "connected"
	if err := s.pool.Ping(ctx); err != nil {
		dbStatus = "disconnected"
		slog.Error("health check: database ping failed", "error", err)
	}

	s3Status := "connected"
	if err := s.s3.HealthCheck(ctx); err != nil {
		s3Status = "disconnected"
		slog.Error("health check: s3 check failed", "error", err)
	}

	natsStatus := "connected"
	if !s.natsOK() {
		natsStatus = "disconnected"
	}

	status := "ok"
	code := http.StatusOK
	if dbStatus != "connected" || s3Status != "connected" || natsStatus != "connected" {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	c.JSON(code, gin.H{
		"status": status,
		"db":     dbStatus,
		"s3":     s3Status,
		"nats":   natsStatus,
	})
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.Handler(),
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down REST server")
		_ = srv.Close()
	}()

	slog.Info("REST server listening", "addr", addr)
	return srv.ListenAndServe()
}

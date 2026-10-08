package rest

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/auth"
	"github.com/fleetml/fleetml/internal/deploy"
	"github.com/fleetml/fleetml/internal/device"
	"github.com/fleetml/fleetml/internal/drift"
	"github.com/fleetml/fleetml/internal/experiment"
	"github.com/fleetml/fleetml/internal/health"
	"github.com/fleetml/fleetml/internal/intelligence"
	"github.com/fleetml/fleetml/internal/metrics"
	"github.com/fleetml/fleetml/internal/registry"
	"github.com/fleetml/fleetml/internal/telemetry"
)

// Server is the REST API server.
type Server struct {
	router        *gin.Engine
	readiness     *health.Checker
	service       *registry.Service
	deviceService *device.Service
	deploySvc     *deploy.Service
	orchestrator  *intelligence.Orchestrator
	telemetrySvc  *telemetry.Service
	experimentSvc *experiment.Service
	driftDetector *drift.Detector
	driftStore    drift.ResultStore
	apiKeys       *auth.APIKeyService
	enrollment    *auth.EnrollmentService
	sessions      *auth.SessionService
	corsOrigins   string
	logger        *slog.Logger
	api           *gin.RouterGroup
}

// NewServer creates a new REST server.
func NewServer(readiness *health.Checker, service *registry.Service, deviceSvc *device.Service, deploySvc *deploy.Service, orchestrator *intelligence.Orchestrator, telemetrySvc *telemetry.Service, experimentSvc *experiment.Service, driftDetector *drift.Detector, driftStore drift.ResultStore, apiKeys *auth.APIKeyService, enrollment *auth.EnrollmentService, sessions *auth.SessionService, corsOrigins string, logger *slog.Logger) *Server {
	if apiKeys == nil {
		panic("rest.NewServer: an API key service is required")
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	s := &Server{
		router:        r,
		readiness:     readiness,
		service:       service,
		deviceService: deviceSvc,
		deploySvc:     deploySvc,
		orchestrator:  orchestrator,
		telemetrySvc:  telemetrySvc,
		experimentSvc: experimentSvc,
		driftDetector: driftDetector,
		driftStore:    driftStore,
		apiKeys:       apiKeys,
		enrollment:    enrollment,
		sessions:      sessions,
		corsOrigins:   corsOrigins,
		logger:        logger,
	}

	// CORS runs first so preflight requests (which carry no API key) are answered.
	r.Use(CORS(s.corsOrigins))
	r.Use(RequestID())
	r.Use(metrics.GinMiddleware())
	r.Use(ErrorRecovery(logger))
	r.Use(RequestLogger(logger))

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Probes and metrics live outside /api/v1 and need no API key.
	s.router.GET("/livez", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	s.router.GET("/readyz", s.ready)
	s.router.GET("/healthz", s.ready) // the original health endpoint, same as /readyz
	s.router.GET("/metrics", gin.WrapH(metrics.Handler()))

	// Everything under /api/v1 except the login requires an admin API key or
	// a dashboard session.
	api := s.router.Group("/api/v1", RequireAuth(s.apiKeys, s.sessions, s.logger))
	s.api = api
	if s.sessions != nil {
		NewSessionHandler(s.sessions, s.logger).RegisterRoutes(s.router.Group("/api/v1"), api)
	}

	NewAPIKeyHandler(s.apiKeys, s.logger).RegisterRoutes(api)
	if s.enrollment != nil {
		NewEnrollmentKeyHandler(s.enrollment, s.logger).RegisterRoutes(api)
	}

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

// EnableSigning adds the signing key routes; call it before serving.
func (s *Server) EnableSigning(h *SigningHandler) {
	h.RegisterRoutes(s.api)
}

// EnableAlertSinks adds the alert sink routes; call it before serving.
func (s *Server) EnableAlertSinks(h *AlertSinkHandler) {
	h.RegisterRoutes(s.api)
}

func (s *Server) Handler() http.Handler {
	return s.router
}

// ready answers 200 when Postgres, S3 and NATS are reachable and 503 otherwise.
func (s *Server) ready(c *gin.Context) {
	statuses, ok := s.readiness.Check(c.Request.Context())
	body := gin.H{"status": "ok"}
	code := http.StatusOK
	if !ok {
		body["status"] = "degraded"
		code = http.StatusServiceUnavailable
	}
	for name, st := range statuses {
		body[name] = st
	}
	c.JSON(code, body)
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

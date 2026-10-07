package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/deploy"
	"github.com/fleetml/fleetml/internal/device"
	"github.com/fleetml/fleetml/internal/intelligence"
)

// IntelligenceHandler handles REST endpoints for the intelligence layer.
type IntelligenceHandler struct {
	orchestrator *intelligence.Orchestrator
	deviceSvc    *device.Service
	deploySvc    *deploy.Service
	logger       *slog.Logger
}

// NewIntelligenceHandler creates a new IntelligenceHandler.
func NewIntelligenceHandler(
	orch *intelligence.Orchestrator,
	deviceSvc *device.Service,
	deploySvc *deploy.Service,
	logger *slog.Logger,
) *IntelligenceHandler {
	return &IntelligenceHandler{
		orchestrator: orch,
		deviceSvc:    deviceSvc,
		deploySvc:    deploySvc,
		logger:       logger,
	}
}

// RegisterRoutes registers intelligence routes.
func (h *IntelligenceHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/devices/:device_id/profile", h.getDeviceProfile)
	rg.POST("/intelligence/preview", h.preview)
}

func (h *IntelligenceHandler) getDeviceProfile(c *gin.Context) {
	id, ok := parseUUID(c, "device_id")
	if !ok {
		return
	}

	dev, err := h.deviceSvc.GetDevice(c.Request.Context(), id)
	if err != nil {
		handleDeviceError(c, err)
		return
	}

	profile := h.orchestrator.GetProfiler().BuildProfile(dev, nil)
	respondData(c, http.StatusOK, profile)
}

type previewRequest struct {
	DeviceID     uuid.UUID `json:"device_id" binding:"required"`
	DeploymentID uuid.UUID `json:"deployment_id" binding:"required"`
}

func (h *IntelligenceHandler) preview(c *gin.Context) {
	var req previewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "device_id and deployment_id are required")
		return
	}

	ctx := c.Request.Context()

	dev, err := h.deviceSvc.GetDevice(ctx, req.DeviceID)
	if err != nil {
		handleDeviceError(c, err)
		return
	}

	dep, err := h.deploySvc.Get(ctx, req.DeploymentID)
	if err != nil {
		handleDeployError(c, err)
		return
	}

	profile := h.orchestrator.GetProfiler().BuildProfile(dev, nil)

	// Parse target selector from deployment
	ts := intelligence.TargetSelector{}
	if dep.TargetSelector != nil {
		ts = parseTargetSelector(dep.TargetSelector)
	}
	targetMatch := ts.Matches(profile, dev.Labels)

	candidates, err := h.orchestrator.LoadVariantsForVersion(ctx, dep.ModelVersionID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load variants")
		return
	}

	selected, allCandidates := h.orchestrator.GetSelector().SelectWithDetails(profile, candidates)

	result := gin.H{
		"device_profile": profile,
		"target_match":   targetMatch,
		"all_candidates": allCandidates,
	}
	if selected != nil {
		result["selected_variant"] = selected
	}

	respondData(c, http.StatusOK, result)
}

func parseTargetSelector(raw map[string]string) intelligence.TargetSelector {
	ts := intelligence.TargetSelector{}
	// The target_selector in deployments is a simple map[string]string used for label matching.
	// For backward compatibility, treat all entries as labels unless they're special keys.
	if len(raw) > 0 {
		ts.Labels = raw
	}
	return ts
}

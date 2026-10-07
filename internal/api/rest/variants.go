package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/registry"
)

type VariantHandler struct {
	service *registry.Service
	logger  *slog.Logger
}

func NewVariantHandler(svc *registry.Service, logger *slog.Logger) *VariantHandler {
	return &VariantHandler{service: svc, logger: logger}
}

func (h *VariantHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/versions/:version_id/variants", h.create)
	rg.GET("/versions/:version_id/variants", h.list)
	rg.GET("/variants/:variant_id", h.get)
	rg.GET("/variants/:variant_id/download", h.download)
	rg.POST("/variants/:variant_id/finalize", h.finalize)
	rg.DELETE("/variants/:variant_id", h.delete)
}

type createVariantRequest struct {
	Quantization       string   `json:"quantization" binding:"required"`
	TargetRuntime      string   `json:"target_runtime" binding:"required"`
	MinRAMMB           int      `json:"min_ram_mb"`
	MinNPUTflops       *float64 `json:"min_npu_tflops"`
	BlobSizeBytes      int64    `json:"blob_size_bytes" binding:"required"`
	BlobSHA256         string   `json:"blob_sha256" binding:"required"`
	BenchmarkLatencyMs *float64 `json:"benchmark_latency_ms"`
	BenchmarkAccuracy  *float64 `json:"benchmark_accuracy"`
}

func (h *VariantHandler) create(c *gin.Context) {
	versionID, ok := parseUUID(c, "version_id")
	if !ok {
		return
	}
	var req createVariantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "quantization, target_runtime, blob_size_bytes, and blob_sha256 are required")
		return
	}

	out, err := h.service.CreateVariant(c.Request.Context(), registry.CreateVariantInput{
		VersionID:          versionID,
		Quantization:       req.Quantization,
		TargetRuntime:      req.TargetRuntime,
		MinRAMMB:           req.MinRAMMB,
		MinNPUTflops:       req.MinNPUTflops,
		BlobSizeBytes:      req.BlobSizeBytes,
		BlobSHA256:         req.BlobSHA256,
		BenchmarkLatencyMs: req.BenchmarkLatencyMs,
		BenchmarkAccuracy:  req.BenchmarkAccuracy,
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": gin.H{
			"variant":    out.Variant,
			"upload_url": out.UploadURL,
		},
	})
}

func (h *VariantHandler) list(c *gin.Context) {
	versionID, ok := parseUUID(c, "version_id")
	if !ok {
		return
	}
	variants, err := h.service.ListVariants(c.Request.Context(), versionID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	if variants == nil {
		variants = []registry.RegistryVariant{}
	}
	respondData(c, http.StatusOK, variants)
}

func (h *VariantHandler) get(c *gin.Context) {
	id, ok := parseUUID(c, "variant_id")
	if !ok {
		return
	}
	v, err := h.service.GetVariant(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusOK, v)
}

func (h *VariantHandler) download(c *gin.Context) {
	id, ok := parseUUID(c, "variant_id")
	if !ok {
		return
	}
	url, err := h.service.GetVariantDownloadURL(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusOK, gin.H{"download_url": url})
}

func (h *VariantHandler) finalize(c *gin.Context) {
	id, ok := parseUUID(c, "variant_id")
	if !ok {
		return
	}
	v, err := h.service.FinalizeVariant(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusOK, v)
}

func (h *VariantHandler) delete(c *gin.Context) {
	id, ok := parseUUID(c, "variant_id")
	if !ok {
		return
	}
	if err := h.service.DeleteVariant(c.Request.Context(), id); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

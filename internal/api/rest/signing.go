package rest

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/signing"
)

// MaxRotationGrace bounds how long a rotated-out key stays valid.
const MaxRotationGrace = 30 * 24 * time.Hour

// SigningHandler exposes a project's artifact signing keys.
type SigningHandler struct {
	keyring *signing.Keyring
	logger  *slog.Logger
}

// NewSigningHandler creates a SigningHandler.
func NewSigningHandler(keyring *signing.Keyring, logger *slog.Logger) *SigningHandler {
	return &SigningHandler{keyring: keyring, logger: logger}
}

// RegisterRoutes registers the signing key routes.
func (h *SigningHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/projects/:project_id/signing-key", h.get)
	rg.POST("/projects/:project_id/signing-key", h.ensure)
	rg.POST("/projects/:project_id/signing-key/rotate", h.rotate)
}

// keysResponse lists the keys devices accept, the active one first.
type keysResponse struct {
	Active   *signing.Key  `json:"active"`
	Keys     []signing.Key `json:"keys"`
	Resigned *int          `json:"resigned_variants,omitempty"`
}

func newKeysResponse(keys []signing.Key) keysResponse {
	r := keysResponse{Keys: keys}
	for i := range keys {
		if keys[i].Status == signing.StatusActive {
			r.Active = &keys[i]
		}
	}
	return r
}

func (h *SigningHandler) get(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	keys, err := h.keyring.Keys(c.Request.Context(), projectID.String())
	if err != nil {
		internalError(c, err, "Failed to load signing keys")
		return
	}
	if len(keys) == 0 {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "the project has no signing key yet; it is created with the first published variant or by POST")
		return
	}
	respondData(c, http.StatusOK, newKeysResponse(keys))
}

// ensure creates the project's key if it has none (publishing a variant does too).
func (h *SigningHandler) ensure(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	if !h.projectExists(c, projectID.String()) {
		return
	}
	keys, err := h.keyring.EnsureKey(c.Request.Context(), projectID.String())
	if err != nil {
		internalError(c, err, "Failed to create the signing key")
		return
	}
	respondData(c, http.StatusOK, newKeysResponse(keys))
}

type rotateRequest struct {
	// Grace is how long devices keep accepting the previous key (default 24h).
	Grace string `json:"grace"`
}

func (h *SigningHandler) rotate(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	var req rotateRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid body")
			return
		}
	}
	grace := 24 * time.Hour
	if req.Grace != "" {
		d, err := time.ParseDuration(req.Grace)
		if err != nil || d < 0 || d > MaxRotationGrace {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "grace must be a duration between 0s and 720h, e.g. \"24h\"")
			return
		}
		grace = d
	}
	if !h.projectExists(c, projectID.String()) {
		return
	}
	newKeyID, resigned, err := h.keyring.Rotate(c.Request.Context(), projectID.String(), grace)
	if err != nil {
		internalError(c, err, "Failed to rotate the signing key")
		return
	}
	h.logger.Info("signing key rotated", "project_id", projectID, "key_id", newKeyID, "grace", grace, "resigned_variants", resigned)
	keys, err := h.keyring.Keys(c.Request.Context(), projectID.String())
	if err != nil {
		internalError(c, err, "Failed to load signing keys")
		return
	}
	r := newKeysResponse(keys)
	r.Resigned = &resigned
	respondData(c, http.StatusOK, r)
}

func (h *SigningHandler) projectExists(c *gin.Context, projectID string) bool {
	ok, err := h.keyring.ProjectExists(c.Request.Context(), projectID)
	if err != nil {
		internalError(c, err, "Failed to load the project")
		return false
	}
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "project not found")
	}
	return ok
}

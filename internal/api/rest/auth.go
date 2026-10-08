package rest

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/auth"
)

const apiKeyIDKey = "api_key_id"

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(header string) (string, bool) {
	const prefix = "bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}

// RequireAPIKey rejects requests without a valid, non-revoked admin API key.
func RequireAPIKey(keys *auth.APIKeyService, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if !ok {
			unauthorized(c, "missing API key: send Authorization: Bearer <key>")
			return
		}
		k, err := keys.Authenticate(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidKey) {
				unauthorized(c, "invalid or revoked API key")
				return
			}
			internalError(c, fmt.Errorf("api key lookup: %w", err), "An internal error occurred")
			c.Abort()
			return
		}
		c.Set(apiKeyIDKey, k.ID.String())
		c.Next()
	}
}

func unauthorized(c *gin.Context, message string) {
	c.Header("WWW-Authenticate", `Bearer realm="fleetml"`)
	respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", message)
	c.Abort()
}

// APIKeyHandler manages admin API keys. Every route requires an existing key.
type APIKeyHandler struct {
	keys   *auth.APIKeyService
	logger *slog.Logger
}

// NewAPIKeyHandler creates an APIKeyHandler.
func NewAPIKeyHandler(keys *auth.APIKeyService, logger *slog.Logger) *APIKeyHandler {
	return &APIKeyHandler{keys: keys, logger: logger}
}

// RegisterRoutes registers API key routes.
func (h *APIKeyHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/api-keys", h.create)
	rg.GET("/api-keys", h.list)
	rg.POST("/api-keys/:key_id/revoke", h.revoke)
}

type createKeyRequest struct {
	Name string `json:"name" binding:"required"`
}

// createdKeyResponse is the only response that ever carries a plaintext key.
type createdKeyResponse struct {
	auth.APIKey
	Key string `json:"key"`
}

func (h *APIKeyHandler) create(c *gin.Context) {
	var req createKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	key, k, err := h.keys.Create(c.Request.Context(), req.Name)
	if err != nil {
		handleAuthError(c, err)
		return
	}
	respondData(c, http.StatusCreated, createdKeyResponse{APIKey: *k, Key: key})
}

func (h *APIKeyHandler) list(c *gin.Context) {
	keys, err := h.keys.List(c.Request.Context())
	if err != nil {
		handleAuthError(c, err)
		return
	}
	respondData(c, http.StatusOK, keys)
}

func (h *APIKeyHandler) revoke(c *gin.Context) {
	id, ok := parseUUID(c, "key_id")
	if !ok {
		return
	}
	if err := h.keys.Revoke(c.Request.Context(), id); err != nil {
		handleAuthError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func handleAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, auth.ErrNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, auth.ErrInvalidInput):
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, auth.ErrLastKey):
		respondError(c, http.StatusConflict, "CONFLICT", "cannot revoke the last active admin key; create another key first")
	default:
		internalError(c, err, "An internal error occurred")
	}
}

// EnrollmentKeyHandler manages per-project device enrollment keys.
type EnrollmentKeyHandler struct {
	keys   *auth.EnrollmentService
	logger *slog.Logger
}

// NewEnrollmentKeyHandler creates an EnrollmentKeyHandler.
func NewEnrollmentKeyHandler(keys *auth.EnrollmentService, logger *slog.Logger) *EnrollmentKeyHandler {
	return &EnrollmentKeyHandler{keys: keys, logger: logger}
}

// RegisterRoutes registers enrollment key routes.
func (h *EnrollmentKeyHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/projects/:project_id/enrollment-keys", h.create)
	rg.GET("/projects/:project_id/enrollment-keys", h.list)
	rg.POST("/enrollment-keys/:key_id/revoke", h.revoke)
	rg.POST("/enrollment-keys/:key_id/rotate", h.rotate)
}

// createdEnrollmentKeyResponse is the only response that carries the plaintext key.
type createdEnrollmentKeyResponse struct {
	auth.EnrollmentKey
	Key string `json:"key"`
}

func (h *EnrollmentKeyHandler) create(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	var req createKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	key, k, err := h.keys.Create(c.Request.Context(), projectID, req.Name)
	if err != nil {
		handleAuthError(c, err)
		return
	}
	respondData(c, http.StatusCreated, createdEnrollmentKeyResponse{EnrollmentKey: *k, Key: key})
}

func (h *EnrollmentKeyHandler) list(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	keys, err := h.keys.List(c.Request.Context(), projectID)
	if err != nil {
		handleAuthError(c, err)
		return
	}
	respondData(c, http.StatusOK, keys)
}

func (h *EnrollmentKeyHandler) revoke(c *gin.Context) {
	id, ok := parseUUID(c, "key_id")
	if !ok {
		return
	}
	if err := h.keys.Revoke(c.Request.Context(), id); err != nil {
		handleAuthError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *EnrollmentKeyHandler) rotate(c *gin.Context) {
	id, ok := parseUUID(c, "key_id")
	if !ok {
		return
	}
	key, k, err := h.keys.Rotate(c.Request.Context(), id)
	if err != nil {
		handleAuthError(c, err)
		return
	}
	respondData(c, http.StatusCreated, createdEnrollmentKeyResponse{EnrollmentKey: *k, Key: key})
}

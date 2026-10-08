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

// RequireAuth accepts an admin API key ("Authorization: Bearer <key>") or a
// dashboard session cookie. Requests with a session cookie that change state
// must also send the session's CSRF token in X-CSRF-Token. sessions may be nil.
func RequireAuth(keys *auth.APIKeyService, sessions *auth.SessionService, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if header := c.GetHeader("Authorization"); header != "" || sessions == nil {
			apiKeyAuth(c, keys, header)
			return
		}
		token, err := c.Cookie(SessionCookie)
		if err != nil || token == "" {
			unauthorized(c, "missing API key: send Authorization: Bearer <key>, or log in to the dashboard")
			return
		}
		sess, err := sessions.Authenticate(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidSession) {
				clearSessionCookie(c)
				c.Header("WWW-Authenticate", `Bearer realm="fleetml"`)
				respondError(c, http.StatusUnauthorized, "SESSION_EXPIRED", "your session expired or was revoked: log in again")
				c.Abort()
				return
			}
			internalError(c, fmt.Errorf("session lookup: %w", err), "An internal error occurred")
			c.Abort()
			return
		}
		if !safeMethod(c.Request.Method) && !sessions.CheckCSRF(token, c.GetHeader(CSRFHeader)) {
			logger.Warn("request with a session cookie and a missing or stale CSRF token rejected",
				"method", c.Request.Method, "path", c.FullPath(), "session_id", sess.ID)
			respondError(c, http.StatusForbidden, "CSRF_INVALID", "missing or stale CSRF token: reload the page")
			c.Abort()
			return
		}
		c.Set(apiKeyIDKey, sess.APIKeyID.String())
		c.Set(sessionKey, sess)
		c.Set(sessionTokenKey, token)
		c.Next()
	}
}

func apiKeyAuth(c *gin.Context, keys *auth.APIKeyService, header string) {
	token, ok := bearerToken(header)
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

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
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
	userID := auth.BootstrapUserID
	if p := principal(c); p != nil {
		userID = p.User.ID // a key for the caller (a global admin); other users' keys: POST /users/{id}/api-keys
	}
	key, k, err := h.keys.Create(c.Request.Context(), userID, req.Name)
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

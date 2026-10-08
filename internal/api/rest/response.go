package rest

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/registry"
)

type paginationResponse struct {
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func respondData(c *gin.Context, status int, data interface{}) {
	c.JSON(status, gin.H{"data": data})
}

func respondList(c *gin.Context, data interface{}, total, limit, offset int) {
	c.JSON(http.StatusOK, gin.H{
		"data": data,
		"pagination": paginationResponse{
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	})
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

// internalError responds 500 without leaking details. The cause is attached to
// the request so RequestLogger logs it together with the request_id, which is
// also returned to the client for correlation.
func internalError(c *gin.Context, err error, message string) {
	if err != nil {
		_ = c.Error(err)
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{
			"code":       "INTERNAL_ERROR",
			"message":    message,
			"request_id": c.GetString(requestIDKey),
		},
	})
}

func handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, registry.ErrNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, registry.ErrConflict):
		respondError(c, http.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, registry.ErrInvalidInput):
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, registry.ErrInvalidTransition):
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, registry.ErrIntegrity):
		respondError(c, http.StatusUnprocessableEntity, "INTEGRITY_ERROR", err.Error())
	default:
		internalError(c, err, "An internal error occurred")
	}
}

func parseUUID(c *gin.Context, param string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(param))
	if err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", param+" must be a valid UUID")
		return uuid.Nil, false
	}
	return id, true
}

func parsePagination(c *gin.Context) (limit, offset int) {
	limit = 20
	offset = 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = n
		}
	}
	return limit, offset
}

package rest

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/fleetml/fleetml/internal/registry"
	"github.com/fleetml/fleetml/internal/telemetry"
)

func setupTelemetryRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// We can't easily mock the telemetry service without a real repo,
	// so test that routes are registered and return proper errors
	svc := telemetry.NewService(nil, nil, nil)
	api := r.Group("/api/v1")
	NewTelemetryHandler(svc, nil).RegisterRoutes(api)
	return r
}

func TestTelemetryRoutes_Registered(t *testing.T) {
	r := setupTelemetryRouter()

	// These will fail at the DB level but confirm routes are wired
	routes := r.Routes()
	paths := make(map[string]bool)
	for _, route := range routes {
		paths[route.Method+" "+route.Path] = true
	}

	assert.True(t, paths["GET /api/v1/telemetry/events"])
	assert.True(t, paths["GET /api/v1/telemetry/summary/:model_version_id"])
	assert.True(t, paths["GET /api/v1/telemetry/timeseries/:model_version_id"])
}

func TestTelemetryEvents_InvalidUUID(t *testing.T) {
	r := setupTelemetryRouter()
	w := doJSON(r, "GET", "/api/v1/telemetry/summary/not-a-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

type fakeVersions map[uuid.UUID]bool

func (f fakeVersions) GetVersion(_ context.Context, id uuid.UUID) (*registry.RegistryVersion, error) {
	if !f[id] {
		return nil, fmt.Errorf("version.GetByID: %w", registry.ErrNotFound)
	}
	return &registry.RegistryVersion{ID: id}, nil
}

func TestTelemetrySummary_UnknownVersionIsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewTelemetryHandler(telemetry.NewService(nil, nil, nil), nil).WithVersions(fakeVersions{}).RegisterRoutes(r.Group("/api/v1"))
	for _, path := range []string{"/summary/", "/summary/%s/by-class", "/timeseries/"} {
		id := uuid.New()
		url := "/api/v1/telemetry" + path + id.String()
		if strings.Contains(path, "%s") {
			url = "/api/v1/telemetry" + fmt.Sprintf(path, id)
		}
		w := doJSON(r, "GET", url, nil)
		assert.Equal(t, http.StatusNotFound, w.Code, url)
		assert.Contains(t, w.Body.String(), `"message":"model version not found"`)
	}
	w := doJSON(r, "GET", "/api/v1/telemetry/summary/not-a-uuid", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

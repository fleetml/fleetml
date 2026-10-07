package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/device"
)

// --- Mocks for device service ---

type mockDeviceRepoREST struct {
	devices      map[uuid.UUID]*device.Device
	fingerprints map[string]uuid.UUID
}

func newMockDeviceRepoREST() *mockDeviceRepoREST {
	return &mockDeviceRepoREST{
		devices:      make(map[uuid.UUID]*device.Device),
		fingerprints: make(map[string]uuid.UUID),
	}
}

func (m *mockDeviceRepoREST) Create(_ context.Context, d *device.Device) error {
	if _, exists := m.fingerprints[d.Fingerprint]; exists {
		return fmt.Errorf("device.Create: %w: fingerprint %q", device.ErrConflict, d.Fingerprint)
	}
	d.ID = uuid.New()
	d.CreatedAt = time.Now()
	d.UpdatedAt = time.Now()
	m.devices[d.ID] = d
	m.fingerprints[d.Fingerprint] = d.ID
	return nil
}

func (m *mockDeviceRepoREST) GetByID(_ context.Context, id uuid.UUID) (*device.Device, error) {
	d, ok := m.devices[id]
	if !ok {
		return nil, fmt.Errorf("device.GetByID: %w", device.ErrNotFound)
	}
	return d, nil
}

func (m *mockDeviceRepoREST) GetByFingerprint(_ context.Context, fp string) (*device.Device, error) {
	id, ok := m.fingerprints[fp]
	if !ok {
		return nil, fmt.Errorf("device.GetByFingerprint: %w", device.ErrNotFound)
	}
	return m.devices[id], nil
}

func (m *mockDeviceRepoREST) List(_ context.Context, limit, offset int) ([]device.Device, int, error) {
	var all []device.Device
	for _, d := range m.devices {
		all = append(all, *d)
	}
	total := len(all)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (m *mockDeviceRepoREST) ListByLabels(_ context.Context, labels map[string]string, limit, offset int) ([]device.Device, int, error) {
	var filtered []device.Device
	for _, d := range m.devices {
		match := true
		for k, v := range labels {
			if d.Labels[k] != v {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, *d)
		}
	}
	total := len(filtered)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

func (m *mockDeviceRepoREST) UpdateLastSeen(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockDeviceRepoREST) UpdateLabels(_ context.Context, _ uuid.UUID, _ map[string]string) error {
	return nil
}
func (m *mockDeviceRepoREST) Update(_ context.Context, d *device.Device) error {
	if _, ok := m.devices[d.ID]; !ok {
		return device.ErrNotFound
	}
	m.devices[d.ID] = d
	return nil
}
func (m *mockDeviceRepoREST) Delete(_ context.Context, id uuid.UUID) error {
	d, ok := m.devices[id]
	if !ok {
		return fmt.Errorf("device.Delete: %w", device.ErrNotFound)
	}
	delete(m.fingerprints, d.Fingerprint)
	delete(m.devices, id)
	return nil
}

type mockDeviceClassRepoREST struct {
	classes map[uuid.UUID]*device.DeviceClass
	names   map[string]uuid.UUID
}

func newMockDeviceClassRepoREST() *mockDeviceClassRepoREST {
	return &mockDeviceClassRepoREST{
		classes: make(map[uuid.UUID]*device.DeviceClass),
		names:   make(map[string]uuid.UUID),
	}
}

func (m *mockDeviceClassRepoREST) Create(_ context.Context, dc *device.DeviceClass) error {
	dc.ID = uuid.New()
	m.classes[dc.ID] = dc
	m.names[dc.Name] = dc.ID
	return nil
}
func (m *mockDeviceClassRepoREST) GetByID(_ context.Context, id uuid.UUID) (*device.DeviceClass, error) {
	dc, ok := m.classes[id]
	if !ok {
		return nil, fmt.Errorf("deviceClass.GetByID: %w", device.ErrNotFound)
	}
	return dc, nil
}
func (m *mockDeviceClassRepoREST) GetByName(_ context.Context, _ string) (*device.DeviceClass, error) {
	return nil, device.ErrNotFound
}
func (m *mockDeviceClassRepoREST) List(_ context.Context) ([]device.DeviceClass, error) {
	var all []device.DeviceClass
	for _, dc := range m.classes {
		all = append(all, *dc)
	}
	return all, nil
}
func (m *mockDeviceClassRepoREST) Upsert(_ context.Context, dc *device.DeviceClass) error {
	if id, exists := m.names[dc.Name]; exists {
		dc.ID = id
		m.classes[id] = dc
		return nil
	}
	dc.ID = uuid.New()
	m.classes[dc.ID] = dc
	m.names[dc.Name] = dc.ID
	return nil
}

type mockDeploymentQuerierREST struct{}

func (m *mockDeploymentQuerierREST) ListActiveForDevice(_ context.Context, _ map[string]string) ([]device.DeploymentInfo, error) {
	return nil, nil
}

type mockVariantQuerierREST struct{}

func (m *mockVariantQuerierREST) ListByVersion(_ context.Context, _ uuid.UUID) ([]device.VariantInfo, error) {
	return nil, nil
}

type mockStorageREST struct{}

func (m *mockStorageREST) GeneratePresignedUpload(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (m *mockStorageREST) GeneratePresignedDownload(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (m *mockStorageREST) ObjectExists(_ context.Context, _ string) (bool, error)   { return true, nil }
func (m *mockStorageREST) DeleteObject(_ context.Context, _ string) error           { return nil }
func (m *mockStorageREST) GetObjectInfo(_ context.Context, _ string) (int64, error) { return 0, nil }

func newDeviceTestService() *device.Service {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return device.NewService(
		newMockDeviceRepoREST(),
		newMockDeviceClassRepoREST(),
		&mockDeploymentQuerierREST{},
		&mockVariantQuerierREST{},
		&mockStorageREST{},
		"test-secret",
		logger,
	)
}

func setupDeviceTestRouter(svc *device.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	NewDeviceHandler(svc, nil).RegisterRoutes(api)
	return r
}

func deviceFingerprint(suffix string) string {
	return device.GenerateFingerprint("ANDROID", "test-chip-"+suffix, 8192)
}

// --- Tests ---

func TestListDevices(t *testing.T) {
	svc := newDeviceTestService()
	r := setupDeviceTestRouter(svc)

	// Register some devices
	for i := 0; i < 3; i++ {
		_, err := svc.RegisterDevice(context.Background(), &device.RegisterRequest{
			Fingerprint: deviceFingerprint(fmt.Sprintf("%d", i)),
			OS:          "ANDROID",
			RAMMB:       8192,
			Labels:      map[string]string{"env": "prod"},
		})
		require.NoError(t, err)
	}

	w := doJSON(r, "GET", "/api/v1/devices", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	assert.NotNil(t, body["pagination"])
}

func TestListDevicesWithLabelFilter(t *testing.T) {
	svc := newDeviceTestService()
	r := setupDeviceTestRouter(svc)

	_, err := svc.RegisterDevice(context.Background(), &device.RegisterRequest{
		Fingerprint: deviceFingerprint("prod"),
		OS:          "ANDROID", RAMMB: 8192,
		Labels: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)
	_, err = svc.RegisterDevice(context.Background(), &device.RegisterRequest{
		Fingerprint: deviceFingerprint("staging"),
		OS:          "ANDROID", RAMMB: 8192,
		Labels: map[string]string{"env": "staging"},
	})
	require.NoError(t, err)

	w := doJSON(r, "GET", "/api/v1/devices?label.env=prod", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	body := parseBody(t, w)
	pagination := body["pagination"].(map[string]interface{})
	assert.Equal(t, float64(1), pagination["total"])
}

func TestGetDevice(t *testing.T) {
	svc := newDeviceTestService()
	r := setupDeviceTestRouter(svc)

	resp, err := svc.RegisterDevice(context.Background(), &device.RegisterRequest{
		Fingerprint: deviceFingerprint("get"),
		OS:          "ANDROID", RAMMB: 8192,
	})
	require.NoError(t, err)

	w := doJSON(r, "GET", "/api/v1/devices/"+resp.DeviceID, nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetDeviceNotFound(t *testing.T) {
	svc := newDeviceTestService()
	r := setupDeviceTestRouter(svc)

	w := doJSON(r, "GET", "/api/v1/devices/"+uuid.New().String(), nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeleteDevice(t *testing.T) {
	svc := newDeviceTestService()
	r := setupDeviceTestRouter(svc)

	resp, err := svc.RegisterDevice(context.Background(), &device.RegisterRequest{
		Fingerprint: deviceFingerprint("del"),
		OS:          "ANDROID", RAMMB: 8192,
	})
	require.NoError(t, err)

	w := doJSON(r, "DELETE", "/api/v1/devices/"+resp.DeviceID, nil)
	assert.Equal(t, http.StatusNoContent, w.Code)

	w = doJSON(r, "GET", "/api/v1/devices/"+resp.DeviceID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestListDeviceClasses(t *testing.T) {
	svc := newDeviceTestService()
	r := setupDeviceTestRouter(svc)

	// Register a device to auto-create a class
	_, err := svc.RegisterDevice(context.Background(), &device.RegisterRequest{
		Fingerprint: deviceFingerprint("class"),
		OS:          "ANDROID", RAMMB: 12288, HasNPU: true,
	})
	require.NoError(t, err)

	w := doJSON(r, "GET", "/api/v1/device-classes", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

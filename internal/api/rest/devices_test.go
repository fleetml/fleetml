package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/auth"
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
	svc := device.NewService(
		newMockDeviceRepoREST(),
		newMockDeviceClassRepoREST(),
		&mockDeploymentQuerierREST{},
		&mockVariantQuerierREST{},
		&mockStorageREST{},
		"test-secret",
		logger,
	)
	svc.SetEnrollmentVerifier(staticEnrollment{})
	return svc
}

const restEnrollKey = "rest-test-enrollment-key"

// staticEnrollment accepts restEnrollKey for a fixed project.
type staticEnrollment struct{}

func (staticEnrollment) VerifyEnrollmentKey(_ context.Context, key string) (uuid.UUID, error) {
	if key == restEnrollKey {
		return uuid.MustParse("33333333-3333-3333-3333-333333333333"), nil
	}
	return uuid.Nil, auth.ErrInvalidKey
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
			EnrollmentKey: restEnrollKey,
			Fingerprint:   deviceFingerprint(fmt.Sprintf("%d", i)),
			OS:            "ANDROID",
			RAMMB:         8192,
			Labels:        map[string]string{"env": "prod"},
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
		EnrollmentKey: restEnrollKey,
		Fingerprint:   deviceFingerprint("prod"),
		OS:            "ANDROID", RAMMB: 8192,
		Labels: map[string]string{"env": "prod"},
	})
	require.NoError(t, err)
	_, err = svc.RegisterDevice(context.Background(), &device.RegisterRequest{
		EnrollmentKey: restEnrollKey,
		Fingerprint:   deviceFingerprint("staging"),
		OS:            "ANDROID", RAMMB: 8192,
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
		EnrollmentKey: restEnrollKey,
		Fingerprint:   deviceFingerprint("get"),
		OS:            "ANDROID", RAMMB: 8192,
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
		EnrollmentKey: restEnrollKey,
		Fingerprint:   deviceFingerprint("del"),
		OS:            "ANDROID", RAMMB: 8192,
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
		EnrollmentKey: restEnrollKey,
		Fingerprint:   deviceFingerprint("class"),
		OS:            "ANDROID", RAMMB: 12288, HasNPU: true,
	})
	require.NoError(t, err)

	w := doJSON(r, "GET", "/api/v1/device-classes", nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

func (m *mockDeviceRepoREST) SetAuthTokenHash(_ context.Context, id uuid.UUID, hash *string) error {
	d, ok := m.devices[id]
	if !ok {
		return fmt.Errorf("device.SetAuthTokenHash: %w", device.ErrNotFound)
	}
	d.AuthTokenHash = hash
	return nil
}

func TestRevokeDevice(t *testing.T) {
	svc := newDeviceTestService()
	r := setupDeviceTestRouter(svc)
	ctx := context.Background()
	resp, err := svc.RegisterDevice(ctx, &device.RegisterRequest{EnrollmentKey: restEnrollKey, Fingerprint: deviceFingerprint("revoke"), OS: "ANDROID"})
	require.NoError(t, err)

	w := doJSON(r, "POST", "/api/v1/devices/"+resp.DeviceID+"/revoke", nil)
	assert.Equal(t, http.StatusNoContent, w.Code)
	_, _, err = svc.ValidateToken(ctx, resp.AuthToken)
	assert.ErrorIs(t, err, device.ErrUnauthenticated)

	assert.Equal(t, http.StatusNotFound, doJSON(r, "POST", "/api/v1/devices/"+uuid.NewString()+"/revoke", nil).Code)
	assert.Equal(t, http.StatusBadRequest, doJSON(r, "POST", "/api/v1/devices/not-a-uuid/revoke", nil).Code)
}

// Regression: GET /devices used to return Fingerprint and AuthTokenHash, which
// let anyone with API access impersonate a device.
func TestDeviceResponses_NoFingerprintOrTokenHash(t *testing.T) {
	svc := newDeviceTestService()
	r := setupDeviceTestRouter(svc)
	fp := deviceFingerprint("dto")
	resp, err := svc.RegisterDevice(context.Background(), &device.RegisterRequest{EnrollmentKey: restEnrollKey, Fingerprint: fp, OS: "ANDROID", RAMMB: 4096})
	require.NoError(t, err)

	for _, path := range []string{"/api/v1/devices", "/api/v1/devices/" + resp.DeviceID} {
		w := doJSON(r, "GET", path, nil)
		require.Equal(t, http.StatusOK, w.Code, path)
		body := w.Body.String()
		assert.NotContains(t, body, fp, path)
		assert.NotContains(t, strings.ToLower(body), "fingerprint", path)
		assert.NotContains(t, strings.ToLower(body), "token", path)
		assert.Contains(t, body, `"ram_mb":4096`, path)
		assert.Contains(t, body, `"id":"`+resp.DeviceID+`"`, path)
	}
}

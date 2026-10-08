package device

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/auth"
)

// --- Mock repositories ---

type mockDeviceRepo struct {
	devices      map[uuid.UUID]*Device
	fingerprints map[string]uuid.UUID
}

func newMockDeviceRepo() *mockDeviceRepo {
	return &mockDeviceRepo{
		devices:      make(map[uuid.UUID]*Device),
		fingerprints: make(map[string]uuid.UUID),
	}
}

func (m *mockDeviceRepo) Create(_ context.Context, d *Device) error {
	if _, exists := m.fingerprints[d.Fingerprint]; exists {
		return fmt.Errorf("device.Create: %w: fingerprint %q", ErrConflict, d.Fingerprint)
	}
	d.ID = uuid.New()
	d.CreatedAt = time.Now()
	d.UpdatedAt = time.Now()
	m.devices[d.ID] = d
	m.fingerprints[d.Fingerprint] = d.ID
	return nil
}

func (m *mockDeviceRepo) GetByID(_ context.Context, id uuid.UUID) (*Device, error) {
	d, ok := m.devices[id]
	if !ok {
		return nil, fmt.Errorf("device.GetByID: %w", ErrNotFound)
	}
	return d, nil
}

func (m *mockDeviceRepo) GetByFingerprint(_ context.Context, fp string) (*Device, error) {
	id, ok := m.fingerprints[fp]
	if !ok {
		return nil, fmt.Errorf("device.GetByFingerprint: %w", ErrNotFound)
	}
	return m.devices[id], nil
}

func (m *mockDeviceRepo) List(_ context.Context, limit, offset int) ([]Device, int, error) {
	var all []Device
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

func (m *mockDeviceRepo) ListByLabels(_ context.Context, labels map[string]string, limit, offset int) ([]Device, int, error) {
	var filtered []Device
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

func (m *mockDeviceRepo) UpdateLastSeen(_ context.Context, id uuid.UUID) error {
	d, ok := m.devices[id]
	if !ok {
		return fmt.Errorf("device.UpdateLastSeen: %w", ErrNotFound)
	}
	now := time.Now()
	d.LastSeenAt = &now
	return nil
}

func (m *mockDeviceRepo) UpdateLabels(_ context.Context, id uuid.UUID, labels map[string]string) error {
	d, ok := m.devices[id]
	if !ok {
		return fmt.Errorf("device.UpdateLabels: %w", ErrNotFound)
	}
	d.Labels = labels
	return nil
}

func (m *mockDeviceRepo) Update(_ context.Context, d *Device) error {
	if _, ok := m.devices[d.ID]; !ok {
		return fmt.Errorf("device.Update: %w", ErrNotFound)
	}
	d.UpdatedAt = time.Now()
	m.devices[d.ID] = d
	return nil
}

func (m *mockDeviceRepo) Delete(_ context.Context, id uuid.UUID) error {
	d, ok := m.devices[id]
	if !ok {
		return fmt.Errorf("device.Delete: %w", ErrNotFound)
	}
	delete(m.fingerprints, d.Fingerprint)
	delete(m.devices, id)
	return nil
}

type mockDeviceClassRepo struct {
	classes map[uuid.UUID]*DeviceClass
	names   map[string]uuid.UUID
}

func newMockDeviceClassRepo() *mockDeviceClassRepo {
	return &mockDeviceClassRepo{
		classes: make(map[uuid.UUID]*DeviceClass),
		names:   make(map[string]uuid.UUID),
	}
}

func (m *mockDeviceClassRepo) Create(_ context.Context, dc *DeviceClass) error {
	if _, exists := m.names[dc.Name]; exists {
		return fmt.Errorf("deviceClass.Create: %w: name %q", ErrConflict, dc.Name)
	}
	dc.ID = uuid.New()
	m.classes[dc.ID] = dc
	m.names[dc.Name] = dc.ID
	return nil
}

func (m *mockDeviceClassRepo) GetByID(_ context.Context, id uuid.UUID) (*DeviceClass, error) {
	dc, ok := m.classes[id]
	if !ok {
		return nil, fmt.Errorf("deviceClass.GetByID: %w", ErrNotFound)
	}
	return dc, nil
}

func (m *mockDeviceClassRepo) GetByName(_ context.Context, name string) (*DeviceClass, error) {
	id, ok := m.names[name]
	if !ok {
		return nil, fmt.Errorf("deviceClass.GetByName: %w", ErrNotFound)
	}
	return m.classes[id], nil
}

func (m *mockDeviceClassRepo) List(_ context.Context) ([]DeviceClass, error) {
	var all []DeviceClass
	for _, dc := range m.classes {
		all = append(all, *dc)
	}
	return all, nil
}

func (m *mockDeviceClassRepo) Upsert(_ context.Context, dc *DeviceClass) error {
	if existingID, exists := m.names[dc.Name]; exists {
		dc.ID = existingID
		m.classes[existingID] = dc
		return nil
	}
	dc.ID = uuid.New()
	m.classes[dc.ID] = dc
	m.names[dc.Name] = dc.ID
	return nil
}

// --- Test helpers ---

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

const (
	testEnrollKey  = "enroll-key-project-a"
	otherEnrollKey = "enroll-key-project-b"
)

var (
	testProjectID  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	otherProjectID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

// fakeEnrollment maps enrollment keys to projects.
type fakeEnrollment map[string]uuid.UUID

func (f fakeEnrollment) VerifyEnrollmentKey(_ context.Context, key string) (uuid.UUID, error) {
	if p, ok := f[key]; ok {
		return p, nil
	}
	return uuid.Nil, auth.ErrInvalidKey
}

func newTestService() (*Service, *mockDeviceRepo) {
	dr := newMockDeviceRepo()
	svc := NewService(dr, newMockDeviceClassRepo(), "test-secret", testLogger())
	svc.SetEnrollmentVerifier(fakeEnrollment{testEnrollKey: testProjectID, otherEnrollKey: otherProjectID})
	return svc, dr
}

func validFingerprint() string {
	return GenerateFingerprint("ANDROID", "snapdragon-8-gen-3", 12288)
}

// --- Tests ---

func TestRegisterDevice_New(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	resp, err := svc.RegisterDevice(ctx, &RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   validFingerprint(),
		OS:            "ANDROID",
		OSVersion:     "14",
		Chipset:       "snapdragon-8-gen-3",
		RAMMB:         12288,
		HasNPU:        true,
		HasGPU:        true,
		NPUTflops:     45.0,
		AgentVersion:  "0.1.0",
		Labels:        map[string]string{"env": "staging"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.DeviceID)
	assert.NotEmpty(t, resp.AuthToken)
	assert.Equal(t, uint32(60), resp.HeartbeatIntervalSecs)
	assert.Equal(t, uint32(100), resp.TelemetryBatchSize)
	assert.Equal(t, uint32(512), resp.MaxCacheSizeMB)
}

func TestRegisterDevice_ReRegister(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	fp := validFingerprint()

	resp1, err := svc.RegisterDevice(ctx, &RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   fp,
		OS:            "ANDROID",
		RAMMB:         12288,
		HasNPU:        true,
	})
	require.NoError(t, err)

	// Re-register with same fingerprint
	resp2, err := svc.RegisterDevice(ctx, &RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   fp,
		OS:            "ANDROID",
		RAMMB:         16384,
		HasNPU:        true,
		AgentVersion:  "0.2.0",
	})
	require.NoError(t, err)

	// Same device ID
	assert.Equal(t, resp1.DeviceID, resp2.DeviceID)
	// Token should still be valid
	assert.NotEmpty(t, resp2.AuthToken)
}

func TestRegisterDevice_InvalidFingerprint(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.RegisterDevice(context.Background(), &RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   "too-short",
		OS:            "ANDROID",
	})
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestRegisterDevice_InvalidOS(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.RegisterDevice(context.Background(), &RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   validFingerprint(),
		OS:            "TEMPOS",
	})
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestRegisterDevice_Classification(t *testing.T) {
	svc, devices := newTestService()
	ctx := context.Background()

	resp, err := svc.RegisterDevice(ctx, &RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   validFingerprint(),
		OS:            "ANDROID",
		RAMMB:         12288,
		HasNPU:        true,
		Chipset:       "snapdragon-8-gen-3",
	})
	require.NoError(t, err)

	// Check device was classified
	deviceID, _ := uuid.Parse(resp.DeviceID)
	d := devices.devices[deviceID]
	assert.NotNil(t, d.DeviceClassID)
}

func TestHeartbeat_NoDeployments(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	// Register device
	regResp, err := svc.RegisterDevice(ctx, &RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   validFingerprint(),
		OS:            "ANDROID",
		RAMMB:         12288,
		HasNPU:        true,
	})
	require.NoError(t, err)

	deviceID, _ := uuid.Parse(regResp.DeviceID)
	resp, err := svc.Heartbeat(ctx, &HeartbeatRequest{
		DeviceID: deviceID,
	})
	require.NoError(t, err)
	assert.Empty(t, resp.Assignments)
	assert.Equal(t, uint32(60), resp.NextHeartbeatSeconds)
}

func TestValidateToken(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	resp, err := svc.RegisterDevice(ctx, &RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   validFingerprint(),
		OS:            "ANDROID",
		RAMMB:         12288,
		HasNPU:        true,
	})
	require.NoError(t, err)

	// Valid token
	deviceID, expiresAt, err := svc.ValidateToken(ctx, resp.AuthToken)
	require.NoError(t, err)
	assert.Equal(t, resp.DeviceID, deviceID.String())
	assert.WithinDuration(t, time.Now().Add(DefaultTokenTTL), expiresAt, time.Minute, "default lifetime is 30 days")
	assert.Equal(t, resp.TokenExpiresAt.Unix(), expiresAt.Unix())

	// Invalid token
	_, _, err = svc.ValidateToken(ctx, "invalid-token")
	assert.ErrorIs(t, err, ErrUnauthenticated)
}

func TestValidateToken_OldTokenRejectedAfterReRegister(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	fp := validFingerprint()

	first, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)
	second, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", CurrentToken: first.AuthToken})
	require.NoError(t, err)

	_, _, err = svc.ValidateToken(ctx, first.AuthToken)
	assert.ErrorIs(t, err, ErrUnauthenticated, "a superseded token must stop working even though its signature is valid")
	_, _, err = svc.ValidateToken(ctx, second.AuthToken)
	assert.NoError(t, err)
}

func TestValidateToken_TokenFromOtherSecretRejected(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	other, _ := newTestService()
	other.tokenSecret = "a-different-secret"
	resp, err := other.RegisterDevice(ctx, &RegisterRequest{Fingerprint: validFingerprint(), OS: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)
	_, _, err = svc.ValidateToken(ctx, resp.AuthToken)
	assert.ErrorIs(t, err, ErrUnauthenticated)
}

func TestRefreshToken(t *testing.T) {
	ctx := context.Background()

	t.Run("too early", func(t *testing.T) {
		svc, _ := newTestService()
		resp, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: validFingerprint(), OS: "ANDROID", EnrollmentKey: testEnrollKey})
		require.NoError(t, err)
		id, exp, err := svc.ValidateToken(ctx, resp.AuthToken)
		require.NoError(t, err)
		_, _, err = svc.RefreshToken(ctx, id, exp)
		assert.ErrorIs(t, err, ErrRefreshTooEarly, "a fresh 30-day token is not refreshable yet")
		_, _, err = svc.ValidateToken(ctx, resp.AuthToken)
		assert.NoError(t, err, "a refused refresh leaves the token valid")
	})

	t.Run("within window", func(t *testing.T) {
		svc, _ := newTestService()
		svc.SetTokenTTL(3 * 24 * time.Hour) // inside the 7-day window from the start
		resp, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: validFingerprint(), OS: "ANDROID", EnrollmentKey: testEnrollKey})
		require.NoError(t, err)
		id, exp, err := svc.ValidateToken(ctx, resp.AuthToken)
		require.NoError(t, err)

		svc.SetTokenTTL(DefaultTokenTTL)
		newToken, newExp, err := svc.RefreshToken(ctx, id, exp)
		require.NoError(t, err)
		assert.NotEqual(t, resp.AuthToken, newToken)
		assert.True(t, newExp.After(exp))

		gotID, _, err := svc.ValidateToken(ctx, newToken)
		require.NoError(t, err)
		assert.Equal(t, id, gotID)
		_, _, err = svc.ValidateToken(ctx, resp.AuthToken)
		assert.ErrorIs(t, err, ErrUnauthenticated, "the refreshed-away token stops working")
	})
}

func TestRevokeDeviceToken(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	fp := validFingerprint()
	resp, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)

	require.NoError(t, svc.RevokeDeviceToken(ctx, uuid.MustParse(resp.DeviceID)))
	_, _, err = svc.ValidateToken(ctx, resp.AuthToken)
	assert.ErrorIs(t, err, ErrUnauthenticated)

	// The revoked token cannot be used to re-register either; an enrollment key can.
	_, err = svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", CurrentToken: resp.AuthToken})
	assert.ErrorIs(t, err, ErrUnauthenticated)
	again, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)
	_, _, err = svc.ValidateToken(ctx, again.AuthToken)
	assert.NoError(t, err)

	assert.ErrorIs(t, svc.RevokeDeviceToken(ctx, uuid.New()), ErrNotFound)
}

func TestListDevices(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		fp := GenerateFingerprint("ANDROID", fmt.Sprintf("chip-%d", i), 8192)
		_, err := svc.RegisterDevice(ctx, &RegisterRequest{
			EnrollmentKey: testEnrollKey,
			Fingerprint:   fp,
			OS:            "ANDROID",
			RAMMB:         8192,
		})
		require.NoError(t, err)
	}

	devices, total, err := svc.ListDevices(ctx, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, devices, 3)
}

func TestDeleteDevice(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()

	resp, err := svc.RegisterDevice(ctx, &RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   validFingerprint(),
		OS:            "ANDROID",
		RAMMB:         8192,
	})
	require.NoError(t, err)

	deviceID, _ := uuid.Parse(resp.DeviceID)
	err = svc.DeleteDevice(ctx, deviceID)
	assert.NoError(t, err)

	_, err = svc.GetDevice(ctx, deviceID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRegisterDevice_RequiresEnrollmentKey(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()

	_, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: validFingerprint(), OS: "ANDROID"})
	assert.ErrorIs(t, err, ErrUnauthenticated, "no key")

	_, err = svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: validFingerprint(), OS: "ANDROID", EnrollmentKey: "wrong"})
	assert.ErrorIs(t, err, ErrUnauthenticated, "unknown key")

	unconfigured := NewService(newMockDeviceRepo(), newMockDeviceClassRepo(), "s", testLogger())
	_, err = unconfigured.RegisterDevice(ctx, &RegisterRequest{Fingerprint: validFingerprint(), OS: "ANDROID", EnrollmentKey: testEnrollKey})
	assert.ErrorIs(t, err, ErrUnauthenticated, "no verifier configured: fail closed")
}

func TestRegisterDevice_EnrollsIntoKeyProject(t *testing.T) {
	ctx := context.Background()
	svc, devices := newTestService()

	resp, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: validFingerprint(), OS: "ANDROID", EnrollmentKey: otherEnrollKey})
	require.NoError(t, err)
	d := devices.devices[uuid.MustParse(resp.DeviceID)]
	require.NotNil(t, d.ProjectID)
	assert.Equal(t, otherProjectID, *d.ProjectID)
}

func TestRegisterDevice_ReRegisterNeedsCredentials(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	fp := validFingerprint()

	first, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)

	// Impersonation attempt: the fingerprint alone is not enough.
	_, err = svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID"})
	assert.ErrorIs(t, err, ErrUnauthenticated)

	// A key for a different project is not enough either.
	_, err = svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", EnrollmentKey: otherEnrollKey})
	assert.ErrorIs(t, err, ErrUnauthenticated)

	// A garbage token is rejected.
	_, err = svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", CurrentToken: "not.a.jwt"})
	assert.ErrorIs(t, err, ErrUnauthenticated)

	// The current token works and yields a new token for the same device.
	second, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", CurrentToken: first.AuthToken})
	require.NoError(t, err)
	assert.Equal(t, first.DeviceID, second.DeviceID)
	assert.NotEqual(t, first.AuthToken, second.AuthToken)

	// The superseded token can no longer be used to re-register.
	_, err = svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", CurrentToken: first.AuthToken})
	assert.ErrorIs(t, err, ErrUnauthenticated)

	// An enrollment key for the device's own project works.
	third, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)
	assert.Equal(t, first.DeviceID, third.DeviceID)

	// Another device's token does not authorize this fingerprint.
	otherFP := GenerateFingerprint("ANDROID", "other-chip", 4096)
	other, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: otherFP, OS: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)
	_, err = svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: fp, OS: "ANDROID", CurrentToken: other.AuthToken})
	assert.ErrorIs(t, err, ErrUnauthenticated)
}

func TestRegisterDevice_LegacyDeviceAdoptsProject(t *testing.T) {
	ctx := context.Background()
	svc, devices := newTestService()
	legacy := &Device{Fingerprint: validFingerprint(), OS: "ANDROID", Labels: map[string]string{}}
	require.NoError(t, devices.Create(ctx, legacy))

	_, err := svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: legacy.Fingerprint, OS: "ANDROID"})
	assert.ErrorIs(t, err, ErrUnauthenticated, "pre-enrollment devices need a key too")

	_, err = svc.RegisterDevice(ctx, &RegisterRequest{Fingerprint: legacy.Fingerprint, OS: "ANDROID", EnrollmentKey: otherEnrollKey})
	require.NoError(t, err)
	require.NotNil(t, devices.devices[legacy.ID].ProjectID)
	assert.Equal(t, otherProjectID, *devices.devices[legacy.ID].ProjectID)
}

func (m *mockDeviceRepo) SetAuthTokenHash(_ context.Context, id uuid.UUID, hash *string) error {
	d, ok := m.devices[id]
	if !ok {
		return fmt.Errorf("device.SetAuthTokenHash: %w", ErrNotFound)
	}
	d.AuthTokenHash = hash
	return nil
}

// fakeComputer returns fixed assignments, or an error.
type fakeComputer struct {
	out []ModelAssignment
	err error
}

func (f fakeComputer) ComputeAssignments(context.Context, *Device, []ModelStatus) ([]ModelAssignment, error) {
	return f.out, f.err
}

func TestHeartbeat_UsesAssignmentComputer(t *testing.T) {
	svc, _ := newTestService()
	ctx := context.Background()
	reg, err := svc.RegisterDevice(ctx, &RegisterRequest{EnrollmentKey: testEnrollKey, Fingerprint: validFingerprint(), OS: "ANDROID"})
	require.NoError(t, err)
	id := uuid.MustParse(reg.DeviceID)

	want := ModelAssignment{ModelID: "m", VersionID: "v", Version: "1.0.0", VariantID: "x", Action: "LOAD"}
	svc.SetAssignmentComputer(fakeComputer{out: []ModelAssignment{want}})
	resp, err := svc.Heartbeat(ctx, &HeartbeatRequest{DeviceID: id})
	require.NoError(t, err)
	assert.Equal(t, []ModelAssignment{want}, resp.Assignments)

	// A failing computer yields no actions (never spurious UNLOADs); the device retries next heartbeat.
	svc.SetAssignmentComputer(fakeComputer{err: errors.New("db down")})
	resp, err = svc.Heartbeat(ctx, &HeartbeatRequest{DeviceID: id})
	require.NoError(t, err)
	assert.Empty(t, resp.Assignments)
}

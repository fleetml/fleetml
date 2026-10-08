package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	grpclib "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	"github.com/fleetml/fleetml/internal/auth"
	"github.com/fleetml/fleetml/internal/device"
)

// --- Mocks for device service dependencies ---

type mockDeviceRepo struct {
	devices      map[uuid.UUID]*device.Device
	fingerprints map[string]uuid.UUID
}

func newMockDeviceRepo() *mockDeviceRepo {
	return &mockDeviceRepo{
		devices:      make(map[uuid.UUID]*device.Device),
		fingerprints: make(map[string]uuid.UUID),
	}
}

func (m *mockDeviceRepo) Create(_ context.Context, d *device.Device) error {
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

func (m *mockDeviceRepo) GetByID(_ context.Context, id uuid.UUID) (*device.Device, error) {
	d, ok := m.devices[id]
	if !ok {
		return nil, fmt.Errorf("device.GetByID: %w", device.ErrNotFound)
	}
	return d, nil
}

func (m *mockDeviceRepo) GetByFingerprint(_ context.Context, fp string) (*device.Device, error) {
	id, ok := m.fingerprints[fp]
	if !ok {
		return nil, fmt.Errorf("device.GetByFingerprint: %w", device.ErrNotFound)
	}
	return m.devices[id], nil
}

func (m *mockDeviceRepo) List(_ context.Context, _, _ int) ([]device.Device, int, error) {
	return nil, 0, nil
}
func (m *mockDeviceRepo) ListByLabels(_ context.Context, _ map[string]string, _, _ int) ([]device.Device, int, error) {
	return nil, 0, nil
}
func (m *mockDeviceRepo) UpdateLastSeen(_ context.Context, id uuid.UUID) error {
	if _, ok := m.devices[id]; !ok {
		return device.ErrNotFound
	}
	return nil
}
func (m *mockDeviceRepo) UpdateLabels(_ context.Context, _ uuid.UUID, _ map[string]string) error {
	return nil
}
func (m *mockDeviceRepo) Update(_ context.Context, d *device.Device) error {
	if _, ok := m.devices[d.ID]; !ok {
		return device.ErrNotFound
	}
	m.devices[d.ID] = d
	return nil
}
func (m *mockDeviceRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.devices[id]; !ok {
		return device.ErrNotFound
	}
	delete(m.devices, id)
	return nil
}

type mockDeviceClassRepo struct {
	classes map[uuid.UUID]*device.DeviceClass
	names   map[string]uuid.UUID
}

func newMockDeviceClassRepo() *mockDeviceClassRepo {
	return &mockDeviceClassRepo{
		classes: make(map[uuid.UUID]*device.DeviceClass),
		names:   make(map[string]uuid.UUID),
	}
}

func (m *mockDeviceClassRepo) Create(_ context.Context, dc *device.DeviceClass) error {
	dc.ID = uuid.New()
	m.classes[dc.ID] = dc
	m.names[dc.Name] = dc.ID
	return nil
}
func (m *mockDeviceClassRepo) GetByID(_ context.Context, id uuid.UUID) (*device.DeviceClass, error) {
	dc, ok := m.classes[id]
	if !ok {
		return nil, device.ErrNotFound
	}
	return dc, nil
}
func (m *mockDeviceClassRepo) GetByName(_ context.Context, name string) (*device.DeviceClass, error) {
	id, ok := m.names[name]
	if !ok {
		return nil, device.ErrNotFound
	}
	return m.classes[id], nil
}
func (m *mockDeviceClassRepo) List(_ context.Context) ([]device.DeviceClass, error) { return nil, nil }
func (m *mockDeviceClassRepo) Upsert(_ context.Context, dc *device.DeviceClass) error {
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

type mockDeploymentQuerier struct{}

func (m *mockDeploymentQuerier) ListActiveForDevice(_ context.Context, _ map[string]string) ([]device.DeploymentInfo, error) {
	return nil, nil
}

type mockVariantQuerier struct{}

func (m *mockVariantQuerier) ListByVersion(_ context.Context, _ uuid.UUID) ([]device.VariantInfo, error) {
	return nil, nil
}

type mockStorageClient struct{}

func (m *mockStorageClient) GeneratePresignedUpload(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", nil
}
func (m *mockStorageClient) GeneratePresignedDownload(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://s3.example.com/" + key, nil
}
func (m *mockStorageClient) ObjectExists(_ context.Context, _ string) (bool, error) {
	return true, nil
}
func (m *mockStorageClient) DeleteObject(_ context.Context, _ string) error { return nil }
func (m *mockStorageClient) GetObjectInfo(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

// --- Test helpers ---

const testEnrollKey = "test-enrollment-key"

var testEnrollProject = uuid.MustParse("11111111-1111-1111-1111-111111111111")

// fakeEnrollment maps enrollment keys to projects.
type fakeEnrollment map[string]uuid.UUID

func (f fakeEnrollment) VerifyEnrollmentKey(_ context.Context, key string) (uuid.UUID, error) {
	if p, ok := f[key]; ok {
		return p, nil
	}
	return uuid.Nil, auth.ErrInvalidKey
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func setupGRPCTest(t *testing.T) fleetmlv1.DeviceServiceClient {
	t.Helper()
	client, _ := setupGRPCTestWithService(t)
	return client
}

func setupGRPCTestWithService(t *testing.T) (fleetmlv1.DeviceServiceClient, *device.Service) {
	t.Helper()
	logger := testLogger()

	deviceSvc := device.NewService(
		newMockDeviceRepo(),
		newMockDeviceClassRepo(),
		&mockDeploymentQuerier{},
		&mockVariantQuerier{},
		&mockStorageClient{},
		"test-secret",
		logger,
	)
	deviceSvc.SetEnrollmentVerifier(fakeEnrollment{testEnrollKey: testEnrollProject})

	// Start in-memory gRPC server
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	gs := grpclib.NewServer(
		grpclib.UnaryInterceptor(AuthInterceptor(deviceSvc)),
	)
	srv := NewDeviceServiceServer(deviceSvc, logger)
	fleetmlv1.RegisterDeviceServiceServer(gs, srv)

	go func() {
		_ = gs.Serve(lis)
	}()
	t.Cleanup(func() { gs.Stop() })

	conn, err := grpclib.NewClient(
		lis.Addr().String(),
		grpclib.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := fleetmlv1.NewDeviceServiceClient(conn)
	return client, deviceSvc
}

func validFingerprint() string {
	return device.GenerateFingerprint("ANDROID", "snapdragon-8-gen-3", 12288)
}

// --- Tests ---

func TestRegister_Success(t *testing.T) {
	client := setupGRPCTest(t)

	resp, err := client.Register(context.Background(), &fleetmlv1.RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   validFingerprint(),
		Os:            "ANDROID",
		OsVersion:     "14",
		Chipset:       "snapdragon-8-gen-3",
		RamMb:         12288,
		HasNpu:        true,
		HasGpu:        true,
		NpuTflops:     45.0,
		AgentVersion:  "0.1.0",
		Labels:        map[string]string{"env": "staging"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.DeviceId)
	assert.NotEmpty(t, resp.AuthToken)
	assert.NotNil(t, resp.Config)
	assert.Equal(t, uint32(60), resp.Config.HeartbeatIntervalSeconds)
}

func TestRegister_DuplicateFingerprint(t *testing.T) {
	client := setupGRPCTest(t)
	fp := validFingerprint()

	resp1, err := client.Register(context.Background(), &fleetmlv1.RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   fp, Os: "ANDROID", RamMb: 12288, HasNpu: true,
	})
	require.NoError(t, err)

	resp2, err := client.Register(context.Background(), &fleetmlv1.RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   fp, Os: "ANDROID", RamMb: 16384, HasNpu: true,
	})
	require.NoError(t, err)
	assert.Equal(t, resp1.DeviceId, resp2.DeviceId)
}

func TestHeartbeat_WithValidToken(t *testing.T) {
	client := setupGRPCTest(t)

	regResp, err := client.Register(context.Background(), &fleetmlv1.RegisterRequest{
		EnrollmentKey: testEnrollKey,
		Fingerprint:   validFingerprint(), Os: "ANDROID", RamMb: 12288, HasNpu: true,
	})
	require.NoError(t, err)

	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+regResp.AuthToken)
	hbResp, err := client.Heartbeat(ctx, &fleetmlv1.HeartbeatRequest{
		DeviceId: regResp.DeviceId,
	})
	require.NoError(t, err)
	assert.Equal(t, uint32(60), hbResp.NextHeartbeatSeconds)
}

func TestHeartbeat_MissingToken(t *testing.T) {
	client := setupGRPCTest(t)

	_, err := client.Heartbeat(context.Background(), &fleetmlv1.HeartbeatRequest{
		DeviceId: uuid.New().String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHeartbeat_InvalidToken(t *testing.T) {
	client := setupGRPCTest(t)

	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer invalid-token")
	_, err := client.Heartbeat(ctx, &fleetmlv1.HeartbeatRequest{
		DeviceId: uuid.New().String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestRegister_WithoutEnrollmentKeyIsUnauthenticated(t *testing.T) {
	client := setupGRPCTest(t)
	_, err := client.Register(context.Background(), &fleetmlv1.RegisterRequest{Fingerprint: validFingerprint(), Os: "ANDROID"})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = client.Register(context.Background(), &fleetmlv1.RegisterRequest{Fingerprint: validFingerprint(), Os: "ANDROID", EnrollmentKey: "nope"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestRegister_InvalidInputIsInvalidArgument(t *testing.T) {
	client := setupGRPCTest(t)
	_, err := client.Register(context.Background(), &fleetmlv1.RegisterRequest{Fingerprint: "short", Os: "ANDROID", EnrollmentKey: testEnrollKey})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestRegister_KnownFingerprintNeedsCredentials(t *testing.T) {
	client := setupGRPCTest(t)
	fp := validFingerprint()
	first, err := client.Register(context.Background(), &fleetmlv1.RegisterRequest{Fingerprint: fp, Os: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)

	_, err = client.Register(context.Background(), &fleetmlv1.RegisterRequest{Fingerprint: fp, Os: "ANDROID"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "fingerprint alone must not return a token")

	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+first.AuthToken)
	second, err := client.Register(ctx, &fleetmlv1.RegisterRequest{Fingerprint: fp, Os: "ANDROID"})
	require.NoError(t, err, "current token in metadata authorizes re-registration")
	assert.Equal(t, first.DeviceId, second.DeviceId)
	assert.NotEqual(t, first.AuthToken, second.AuthToken)
}

func (m *mockDeviceRepo) SetAuthTokenHash(_ context.Context, id uuid.UUID, hash *string) error {
	d, ok := m.devices[id]
	if !ok {
		return device.ErrNotFound
	}
	d.AuthTokenHash = hash
	return nil
}

func withToken(token string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func TestHeartbeat_OldTokenRejectedAfterReRegister(t *testing.T) {
	client := setupGRPCTest(t)
	fp := validFingerprint()
	first, err := client.Register(context.Background(), &fleetmlv1.RegisterRequest{Fingerprint: fp, Os: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)
	_, err = client.Heartbeat(withToken(first.AuthToken), &fleetmlv1.HeartbeatRequest{DeviceId: first.DeviceId})
	require.NoError(t, err)

	second, err := client.Register(withToken(first.AuthToken), &fleetmlv1.RegisterRequest{Fingerprint: fp, Os: "ANDROID"})
	require.NoError(t, err)

	_, err = client.Heartbeat(withToken(first.AuthToken), &fleetmlv1.HeartbeatRequest{DeviceId: first.DeviceId})
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "old token must fail after re-register")
	_, err = client.Heartbeat(withToken(second.AuthToken), &fleetmlv1.HeartbeatRequest{DeviceId: second.DeviceId})
	assert.NoError(t, err)
}

func TestRefreshToken_RPC(t *testing.T) {
	client, svc := setupGRPCTestWithService(t)

	reg, err := client.Register(context.Background(), &fleetmlv1.RegisterRequest{Fingerprint: validFingerprint(), Os: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)
	assert.InDelta(t, time.Now().Add(device.DefaultTokenTTL).Unix(), reg.TokenExpiresAt, 60)

	_, err = client.RefreshToken(withToken(reg.AuthToken), &fleetmlv1.RefreshTokenRequest{})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err), "30 days left: too early")

	_, err = client.RefreshToken(context.Background(), &fleetmlv1.RefreshTokenRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "refresh needs a token")

	// A short-lived token is inside the refresh window.
	svc.SetTokenTTL(2 * 24 * time.Hour)
	short, err := client.Register(withToken(reg.AuthToken), &fleetmlv1.RegisterRequest{Fingerprint: validFingerprint(), Os: "ANDROID"})
	require.NoError(t, err)
	svc.SetTokenTTL(device.DefaultTokenTTL)

	refreshed, err := client.RefreshToken(withToken(short.AuthToken), &fleetmlv1.RefreshTokenRequest{})
	require.NoError(t, err)
	assert.Greater(t, refreshed.TokenExpiresAt, short.TokenExpiresAt)

	_, err = client.Heartbeat(withToken(refreshed.AuthToken), &fleetmlv1.HeartbeatRequest{DeviceId: short.DeviceId})
	assert.NoError(t, err)
	_, err = client.Heartbeat(withToken(short.AuthToken), &fleetmlv1.HeartbeatRequest{DeviceId: short.DeviceId})
	assert.Equal(t, codes.Unauthenticated, status.Code(err), "refreshed-away token fails")
}

func TestHeartbeat_RevokedToken(t *testing.T) {
	client, svc := setupGRPCTestWithService(t)
	reg, err := client.Register(context.Background(), &fleetmlv1.RegisterRequest{Fingerprint: validFingerprint(), Os: "ANDROID", EnrollmentKey: testEnrollKey})
	require.NoError(t, err)

	require.NoError(t, svc.RevokeDeviceToken(context.Background(), uuid.MustParse(reg.DeviceId)))
	_, err = client.Heartbeat(withToken(reg.AuthToken), &fleetmlv1.HeartbeatRequest{DeviceId: reg.DeviceId})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

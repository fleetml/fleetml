package grpc

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	"github.com/fleetml/fleetml/internal/device"
)

type contextKey string

const deviceIDKey contextKey = "device_id"

// DeviceServiceServer implements the fleetmlv1.DeviceServiceServer interface.
type DeviceServiceServer struct {
	fleetmlv1.UnimplementedDeviceServiceServer
	service *device.Service
	logger  *slog.Logger
}

// NewDeviceServiceServer creates a new DeviceServiceServer.
func NewDeviceServiceServer(svc *device.Service, logger *slog.Logger) *DeviceServiceServer {
	return &DeviceServiceServer{service: svc, logger: logger}
}

// Register handles device registration.
func (s *DeviceServiceServer) Register(ctx context.Context, req *fleetmlv1.RegisterRequest) (*fleetmlv1.RegisterResponse, error) {
	resp, err := s.service.RegisterDevice(ctx, &device.RegisterRequest{
		Fingerprint:  req.GetFingerprint(),
		OS:           req.GetOs(),
		OSVersion:    req.GetOsVersion(),
		Chipset:      req.GetChipset(),
		RAMMB:        int(req.GetRamMb()),
		HasNPU:       req.GetHasNpu(),
		HasGPU:       req.GetHasGpu(),
		NPUTflops:    float64(req.GetNpuTflops()),
		AgentVersion: req.GetAgentVersion(),
		Labels:       req.GetLabels(),
	})
	if err != nil {
		s.logger.Error("device registration failed", "fingerprint", req.GetFingerprint(), "error", err)
		return nil, status.Errorf(codes.Internal, "registration failed: %v", err)
	}

	return &fleetmlv1.RegisterResponse{
		DeviceId:  resp.DeviceID,
		AuthToken: resp.AuthToken,
		Config: &fleetmlv1.DeviceConfig{
			HeartbeatIntervalSeconds: resp.HeartbeatIntervalSecs,
			TelemetryBatchSize:       resp.TelemetryBatchSize,
			MaxCacheSizeMb:           resp.MaxCacheSizeMB,
		},
	}, nil
}

// Heartbeat handles device heartbeat.
func (s *DeviceServiceServer) Heartbeat(ctx context.Context, req *fleetmlv1.HeartbeatRequest) (*fleetmlv1.HeartbeatResponse, error) {
	// Device ID should be in context from auth interceptor
	deviceID, ok := ctx.Value(deviceIDKey).(uuid.UUID)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "missing device authentication")
	}

	var loadedModels []device.ModelStatus
	for _, m := range req.GetLoadedModels() {
		loadedModels = append(loadedModels, device.ModelStatus{
			ModelID:   m.GetModelId(),
			Version:   m.GetVersion(),
			VariantID: m.GetVariantId(),
			Status:    m.GetStatus(),
		})
	}

	resp, err := s.service.Heartbeat(ctx, &device.HeartbeatRequest{
		DeviceID:     deviceID,
		LoadedModels: loadedModels,
	})
	if err != nil {
		s.logger.Error("heartbeat failed", "device_id", deviceID, "error", err)
		return nil, status.Errorf(codes.Internal, "heartbeat failed: %v", err)
	}

	var protoAssignments []*fleetmlv1.ModelAssignment
	for _, a := range resp.Assignments {
		action := fleetmlv1.ModelAssignment_ACTION_UNSPECIFIED
		switch a.Action {
		case "LOAD":
			action = fleetmlv1.ModelAssignment_LOAD
		case "UPDATE":
			action = fleetmlv1.ModelAssignment_UPDATE
		case "UNLOAD":
			action = fleetmlv1.ModelAssignment_UNLOAD
		}
		protoAssignments = append(protoAssignments, &fleetmlv1.ModelAssignment{
			ModelId:       a.ModelID,
			ModelName:     a.ModelName,
			Version:       a.Version,
			VariantId:     a.VariantID,
			DownloadUrl:   a.DownloadURL,
			BlobSizeBytes: a.BlobSizeBytes,
			BlobSha256:    a.BlobSHA256,
			Action:        action,
		})
	}

	return &fleetmlv1.HeartbeatResponse{
		Assignments:          protoAssignments,
		NextHeartbeatSeconds: resp.NextHeartbeatSeconds,
	}, nil
}

// AuthInterceptor returns a gRPC unary interceptor that validates JWT tokens.
// Register is exempt from auth.
func AuthInterceptor(svc *device.Service) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip auth for Register
		if info.FullMethod == fleetmlv1.DeviceService_Register_FullMethodName {
			return handler(ctx, req)
		}

		// Require auth for device and telemetry services
		requiresAuth := strings.HasPrefix(info.FullMethod, "/fleetml.device.v1.") ||
			strings.HasPrefix(info.FullMethod, "/fleetml.telemetry.v1.")
		if !requiresAuth {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Errorf(codes.Unauthenticated, "missing metadata")
		}

		authHeader := md.Get("authorization")
		if len(authHeader) == 0 {
			return nil, status.Errorf(codes.Unauthenticated, "missing authorization header")
		}

		token := strings.TrimPrefix(authHeader[0], "Bearer ")
		token = strings.TrimPrefix(token, "bearer ")
		if token == authHeader[0] {
			return nil, status.Errorf(codes.Unauthenticated, "invalid authorization format, expected Bearer token")
		}

		deviceID, err := svc.ValidateToken(token)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}

		ctx = context.WithValue(ctx, deviceIDKey, deviceID)
		return handler(ctx, req)
	}
}

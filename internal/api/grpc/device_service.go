package grpc

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	"github.com/fleetml/fleetml/internal/device"
)

type contextKey string

const (
	deviceIDKey    contextKey = "device_id"
	tokenExpiryKey contextKey = "token_expires_at"
)

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
		Fingerprint:   req.GetFingerprint(),
		OS:            req.GetOs(),
		OSVersion:     req.GetOsVersion(),
		Chipset:       req.GetChipset(),
		RAMMB:         int(req.GetRamMb()),
		HasNPU:        req.GetHasNpu(),
		HasGPU:        req.GetHasGpu(),
		NPUTflops:     float64(req.GetNpuTflops()),
		AgentVersion:  req.GetAgentVersion(),
		Labels:        req.GetLabels(),
		EnrollmentKey: req.GetEnrollmentKey(),
		CurrentToken:  bearerFromMetadata(ctx),
	})
	if err != nil {
		switch {
		case errors.Is(err, device.ErrInvalidInput):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, device.ErrUnauthenticated):
			s.logger.Warn("device registration rejected", "error", err)
			return nil, status.Error(codes.Unauthenticated, err.Error())
		}
		s.logger.Error("device registration failed", "error", err)
		return nil, status.Error(codes.Internal, "registration failed")
	}

	return &fleetmlv1.RegisterResponse{
		DeviceId:       resp.DeviceID,
		AuthToken:      resp.AuthToken,
		TokenExpiresAt: resp.TokenExpiresAt.Unix(),
		Config: &fleetmlv1.DeviceConfig{
			HeartbeatIntervalSeconds:      resp.HeartbeatIntervalSecs,
			TelemetryBatchSize:            resp.TelemetryBatchSize,
			TelemetryFlushIntervalSeconds: resp.TelemetryFlushSecs,
			MaxCacheSizeMb:                resp.MaxCacheSizeMB,
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

	if id := req.GetDeviceId(); id != "" && id != deviceID.String() {
		return nil, status.Error(codes.InvalidArgument, "device_id does not match the authenticated device")
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
		if errors.Is(err, device.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "device not found")
		}
		s.logger.Error("heartbeat failed", "device_id", deviceID, "error", err)
		return nil, status.Error(codes.Internal, "heartbeat failed")
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
			VersionId:     a.VersionID,
			ModelName:     a.ModelName,
			Version:       a.Version,
			VariantId:     a.VariantID,
			DownloadUrl:   a.DownloadURL,
			BlobSizeBytes: a.BlobSizeBytes,
			BlobSha256:    a.BlobSHA256,
			Action:        action,
		})
	}

	protoExperiments := make([]*fleetmlv1.ExperimentAssignment, 0, len(resp.Experiments))
	for _, e := range resp.Experiments {
		protoExperiments = append(protoExperiments, &fleetmlv1.ExperimentAssignment{
			ExperimentId:   e.ExperimentID,
			ArmName:        e.ArmName,
			ArmId:          e.ArmID,
			ModelVersionId: e.ModelVersionID,
		})
	}

	return &fleetmlv1.HeartbeatResponse{
		Assignments:          protoAssignments,
		Experiments:          protoExperiments,
		NextHeartbeatSeconds: resp.NextHeartbeatSeconds,
	}, nil
}

// RefreshToken exchanges a token close to expiry for a new one.
func (s *DeviceServiceServer) RefreshToken(ctx context.Context, _ *fleetmlv1.RefreshTokenRequest) (*fleetmlv1.RefreshTokenResponse, error) {
	deviceID, ok := ctx.Value(deviceIDKey).(uuid.UUID)
	expiry, ok2 := ctx.Value(tokenExpiryKey).(time.Time)
	if !ok || !ok2 {
		return nil, status.Errorf(codes.Unauthenticated, "missing device authentication")
	}
	token, expiresAt, err := s.service.RefreshToken(ctx, deviceID, expiry)
	if err != nil {
		if errors.Is(err, device.ErrRefreshTooEarly) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}
		s.logger.Error("token refresh failed", "device_id", deviceID, "error", err)
		return nil, status.Error(codes.Internal, "token refresh failed")
	}
	return &fleetmlv1.RefreshTokenResponse{AuthToken: token, TokenExpiresAt: expiresAt.Unix()}, nil
}

// bearerFromMetadata returns the bearer token from "authorization" metadata, or "".
func bearerFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return ""
	}
	const prefix = "bearer "
	if len(values[0]) <= len(prefix) || !strings.EqualFold(values[0][:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(values[0][len(prefix):])
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

		deviceID, expiresAt, err := svc.ValidateToken(ctx, token)
		if err != nil {
			if errors.Is(err, device.ErrUnauthenticated) {
				return nil, status.Error(codes.Unauthenticated, err.Error())
			}
			slog.Error("token validation failed", "method", info.FullMethod, "error", err)
			return nil, status.Error(codes.Internal, "token validation failed")
		}

		ctx = context.WithValue(ctx, deviceIDKey, deviceID)
		ctx = context.WithValue(ctx, tokenExpiryKey, expiresAt)
		return handler(ctx, req)
	}
}

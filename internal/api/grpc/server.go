package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"time"

	"google.golang.org/grpc"
	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

// Server wraps a gRPC server with the standard health service.
type Server struct {
	grpcServer *grpc.Server
	health     *grpchealth.Server
}

// keepalivePolicy lets clients ping an idle connection every 20 s or more:
// the Linux agent pings after 30 s without traffic to find a dead connection.
// gRPC's default (5 minutes, and never without an open stream) would close
// such a client's connection with "too_many_pings".
var keepalivePolicy = keepalive.EnforcementPolicy{MinTime: 20 * time.Second, PermitWithoutStream: true}

// NewServer creates a gRPC server with the given server options and the
// grpc.health.v1 service. Reflection (for grpcurl and similar tools) is
// registered only if enabled.
func NewServer(enableReflection bool, opts ...grpc.ServerOption) *Server {
	gs := grpc.NewServer(append([]grpc.ServerOption{grpc.KeepaliveEnforcementPolicy(keepalivePolicy)}, opts...)...)
	hs := grpchealth.NewServer()
	healthpb.RegisterHealthServer(gs, hs)
	if enableReflection {
		reflection.Register(gs)
	}
	return &Server{grpcServer: gs, health: hs}
}

// WatchReadiness sets the health status ("" and each FleetML service) from
// ready every interval until ctx ends.
func (s *Server) WatchReadiness(ctx context.Context, ready func(context.Context) bool, interval time.Duration) {
	for {
		status := healthpb.HealthCheckResponse_NOT_SERVING
		if ready(ctx) {
			status = healthpb.HealthCheckResponse_SERVING
		}
		for _, svc := range []string{"", fleetmlv1.DeviceService_ServiceDesc.ServiceName, fleetmlv1.TelemetryService_ServiceDesc.ServiceName} {
			s.health.SetServingStatus(svc, status)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// GRPCServer returns the underlying grpc.Server for service registration.
func (s *Server) GRPCServer() *grpc.Server {
	return s.grpcServer
}

// ListenAndServe starts listening on the given address.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down gRPC server")
		s.health.Shutdown()
		s.grpcServer.GracefulStop()
	}()

	slog.Info("gRPC server listening", "addr", addr)
	return s.grpcServer.Serve(lis)
}

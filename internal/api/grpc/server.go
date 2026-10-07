package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Server wraps a gRPC server.
type Server struct {
	grpcServer *grpc.Server
}

// NewServer creates a gRPC server with the given server options.
func NewServer(opts ...grpc.ServerOption) *Server {
	gs := grpc.NewServer(opts...)
	reflection.Register(gs)
	return &Server{grpcServer: gs}
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
		s.grpcServer.GracefulStop()
	}()

	slog.Info("gRPC server listening", "addr", addr)
	return s.grpcServer.Serve(lis)
}

package grpchealth

import (
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Server wraps a gRPC health service.
type Server struct {
	grpc   *grpc.Server
	health *health.Server
	lis    net.Listener
	logger *slog.Logger
}

// Start listens on addr (e.g. ":8081") and serves grpc.health.v1.Health.
func Start(addr string, logger *slog.Logger) (*Server, error) {
	if logger == nil {
		logger = slog.Default()
	}
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("grpc listen %s: %w", addr, err)
	}
	gs := grpc.NewServer()
	hs := health.NewServer()
	healthpb.RegisterHealthServer(gs, hs)
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	hs.SetServingStatus("mcp-gateway", healthpb.HealthCheckResponse_SERVING)

	s := &Server{grpc: gs, health: hs, lis: lis, logger: logger}
	go func() {
		logger.Info("grpc health listening", "addr", addr)
		if err := gs.Serve(lis); err != nil {
			logger.Error("grpc serve stopped", "error", err)
		}
	}()
	return s, nil
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop() {
	if s == nil || s.grpc == nil {
		return
	}
	s.health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	s.grpc.GracefulStop()
}

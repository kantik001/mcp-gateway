package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kantik001/mcp-gateway/internal/config"
	"github.com/kantik001/mcp-gateway/internal/grpchealth"
	"github.com/kantik001/mcp-gateway/internal/otelx"
	"github.com/kantik001/mcp-gateway/internal/proxy"
	"github.com/kantik001/mcp-gateway/internal/registry"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serviceName := envOr("OTEL_SERVICE_NAME", "mcp-gateway")
	otelShutdown, err := otelx.Setup(ctx, serviceName, cfg.OTELEndpoint)
	if err != nil {
		logger.Error("otel setup failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		if err := otelShutdown(shutdownCtx); err != nil {
			logger.Error("otel shutdown error", "error", err)
		}
	}()

	reg := registry.NewMemory(logger)
	registry.LoadFromConfig(reg, cfg.Servers, logger)
	registry.StartHealthLoop(ctx, reg, cfg.HealthCheckInterval, logger)

	h := proxy.NewWithOptions(reg, logger, cfg.APIKey, cfg.ToolCallTimeout, cfg.DefaultTenant)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	grpcSrv, err := grpchealth.Start(":"+cfg.GRPCPort, logger)
	if err != nil {
		logger.Error("grpc health start failed", "error", err)
		os.Exit(1)
	}

	go func() {
		logger.Info("mcp-gateway listening", "addr", srv.Addr, "grpc", ":"+cfg.GRPCPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stop
	logger.Info("shutdown signal received", "signal", sig.String())

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown error", "error", err)
	}
	grpcSrv.Stop()
	if err := reg.Close(); err != nil {
		logger.Error("registry close error", "error", err)
	}
	logger.Info("shutdown complete")
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

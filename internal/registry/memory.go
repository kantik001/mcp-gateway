package registry

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kantik001/mcp-gateway/internal/config"
	"github.com/kantik001/mcp-gateway/internal/mcp"
	"github.com/kantik001/mcp-gateway/internal/metrics"
)

type entry struct {
	cfg     config.ServerConfig
	client  *mcp.Client
	healthy bool
}

// MemoryRegistry is an in-memory Registry for MVP.
type MemoryRegistry struct {
	mu      sync.RWMutex
	servers map[string]*entry
	logger  *slog.Logger
}

// NewMemory creates an empty in-memory registry.
func NewMemory(logger *slog.Logger) *MemoryRegistry {
	if logger == nil {
		logger = slog.Default()
	}
	return &MemoryRegistry{
		servers: make(map[string]*entry),
		logger:  logger,
	}
}

// Register starts an MCP server process, initializes it, and stores the client.
func (r *MemoryRegistry) Register(server config.ServerConfig) error {
	if server.Name == "" {
		return fmt.Errorf("server name is required")
	}
	if !server.Enabled {
		r.mu.Lock()
		r.servers[server.Name] = &entry{cfg: server, healthy: false}
		r.mu.Unlock()
		metrics.ServerUp.WithLabelValues(server.Name).Set(0)
		r.logger.Info("server disabled, skipping start", "name", server.Name)
		return nil
	}

	env := make([]string, 0, len(server.Env))
	for k, v := range server.Env {
		env = append(env, k+"="+v)
	}

	transport, err := mcp.NewStdioTransport(mcp.StdioConfig{
		Command: server.Command,
		Args:    server.Args,
		Env:     env,
		Logger:  r.logger.With("mcp_server", server.Name),
	})
	if err != nil {
		r.mu.Lock()
		r.servers[server.Name] = &entry{cfg: server, healthy: false}
		r.mu.Unlock()
		metrics.ServerUp.WithLabelValues(server.Name).Set(0)
		return fmt.Errorf("start server %s: %w", server.Name, err)
	}

	client := mcp.NewClient(server.Name, transport, mcp.WithLogger(r.logger.With("mcp_server", server.Name)))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if _, err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		r.mu.Lock()
		r.servers[server.Name] = &entry{cfg: server, healthy: false}
		r.mu.Unlock()
		metrics.ServerUp.WithLabelValues(server.Name).Set(0)
		return fmt.Errorf("initialize server %s: %w", server.Name, err)
	}

	r.mu.Lock()
	// Replace previous instance if any.
	if prev, ok := r.servers[server.Name]; ok && prev.client != nil {
		_ = prev.client.Close()
	}
	r.servers[server.Name] = &entry{cfg: server, client: client, healthy: true}
	r.mu.Unlock()
	metrics.ServerUp.WithLabelValues(server.Name).Set(1)
	r.logger.Info("registered mcp server", "name", server.Name)
	return nil
}

// Get returns a healthy client by name.
func (r *MemoryRegistry) Get(name string) (*mcp.Client, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.servers[name]
	if !ok {
		return nil, fmt.Errorf("server %q not found", name)
	}
	if e.client == nil {
		return nil, fmt.Errorf("server %q is not available", name)
	}
	return e.client, nil
}

// List returns all registered servers.
func (r *MemoryRegistry) List() []ServerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ServerInfo, 0, len(r.servers))
	for _, e := range r.servers {
		out = append(out, ServerInfo{
			Name:        e.cfg.Name,
			Description: e.cfg.Description,
			Healthy:     e.healthy,
			Command:     e.cfg.Command,
			Enabled:     e.cfg.Enabled,
		})
	}
	return out
}

// HealthCheck pings each enabled server and updates healthy flags.
func (r *MemoryRegistry) HealthCheck(ctx context.Context) map[string]bool {
	r.mu.RLock()
	snapshot := make([]*entry, 0, len(r.servers))
	for _, e := range r.servers {
		snapshot = append(snapshot, e)
	}
	r.mu.RUnlock()

	result := make(map[string]bool, len(snapshot))
	for _, e := range snapshot {
		if e.client == nil || !e.cfg.Enabled {
			result[e.cfg.Name] = false
			r.setHealthy(e.cfg.Name, false)
			continue
		}
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := e.client.Ping(checkCtx)
		cancel()
		ok := err == nil
		if err != nil {
			r.logger.Warn("health check failed", "name", e.cfg.Name, "error", err)
		}
		result[e.cfg.Name] = ok
		r.setHealthy(e.cfg.Name, ok)
	}
	return result
}

func (r *MemoryRegistry) setHealthy(name string, healthy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.servers[name]; ok {
		e.healthy = healthy
	}
	if healthy {
		metrics.ServerUp.WithLabelValues(name).Set(1)
	} else {
		metrics.ServerUp.WithLabelValues(name).Set(0)
	}
}

// Close shuts down all clients.
func (r *MemoryRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for name, e := range r.servers {
		if e.client != nil {
			if err := e.client.Close(); err != nil && firstErr == nil {
				firstErr = err
				r.logger.Warn("error closing client", "name", name, "error", err)
			}
			e.client = nil
		}
		e.healthy = false
		metrics.ServerUp.WithLabelValues(name).Set(0)
	}
	return firstErr
}

// StartHealthLoop runs periodic health checks until ctx is cancelled.
func StartHealthLoop(ctx context.Context, reg Registry, interval time.Duration, logger *slog.Logger) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status := reg.HealthCheck(ctx)
				logger.Debug("health check complete", "status", status)
			}
		}
	}()
}

// LoadFromConfig registers all servers from config. Failures are logged; gateway stays up.
func LoadFromConfig(reg Registry, servers []config.ServerConfig, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	for _, s := range servers {
		if err := reg.Register(s); err != nil {
			logger.Error("failed to register mcp server", "name", s.Name, "error", err)
		}
	}
}

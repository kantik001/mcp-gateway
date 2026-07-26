package registry

import (
	"context"

	"github.com/kantik001/mcp-gateway/internal/config"
	"github.com/kantik001/mcp-gateway/internal/mcp"
)

// ServerInfo is a public view of a registered MCP server.
type ServerInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Healthy     bool   `json:"healthy"`
	Command     string `json:"command"`
	Enabled     bool   `json:"enabled"`
}

// Registry manages MCP server clients.
type Registry interface {
	Register(server config.ServerConfig) error
	Get(name string) (*mcp.Client, error)
	List() []ServerInfo
	HealthCheck(ctx context.Context) map[string]bool
	Close() error
}

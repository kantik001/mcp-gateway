package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kantik001/mcp-gateway/internal/config"
)

func TestLoadYAMLAndEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "servers.yaml")
	content := `
servers:
  - name: filesystem
    description: fs
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/data"]
    enabled: true
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CONFIG_PATH", path)
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("HEALTH_CHECK_INTERVAL", "15s")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "9090" {
		t.Fatalf("port=%s", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("log=%s", cfg.LogLevel)
	}
	if cfg.HealthCheckInterval.Seconds() != 15 {
		t.Fatalf("interval=%v", cfg.HealthCheckInterval)
	}
	if len(cfg.Servers) != 1 || cfg.Servers[0].Name != "filesystem" {
		t.Fatalf("servers=%+v", cfg.Servers)
	}
}

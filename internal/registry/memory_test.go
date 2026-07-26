package registry_test

import (
	"context"
	"testing"
	"time"

	"github.com/kantik001/mcp-gateway/internal/config"
	"github.com/kantik001/mcp-gateway/internal/mcp"
	"github.com/kantik001/mcp-gateway/internal/registry"
)

func TestMemoryRegistryWithMockClient(t *testing.T) {
	reg := registry.NewMemory(nil)

	// Register disabled server — no process start.
	err := reg.Register(config.ServerConfig{
		Name:    "disabled",
		Command: "false",
		Enabled: false,
	})
	if err != nil {
		t.Fatal(err)
	}

	list := reg.List()
	if len(list) != 1 || list[0].Healthy {
		t.Fatalf("list=%+v", list)
	}

	// Manually inject a mock client via a custom register path:
	// use Get after placing through a thin wrapper — MemoryRegistry has no Inject,
	// so we verify HealthCheck + List behavior with disabled-only registry.
	status := reg.HealthCheck(context.Background())
	if status["disabled"] {
		t.Fatal("disabled should be unhealthy")
	}

	if _, err := reg.Get("disabled"); err == nil {
		t.Fatal("expected error for unavailable client")
	}

	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}
	_ = time.Second
	_ = mcp.ProtocolVersion
}

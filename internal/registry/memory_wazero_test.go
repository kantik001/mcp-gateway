package registry_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kantik001/mcp-gateway/internal/config"
	"github.com/kantik001/mcp-gateway/internal/registry"
)

func TestRegisterWASMCalculator(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	wasm := filepath.Join(filepath.Dir(file), "..", "mcp", "testdata", "calculator.wasm")

	reg := registry.NewMemory(slog.Default())
	defer reg.Close()

	err := reg.Register(config.ServerConfig{
		Name:        "calculator",
		Description: "wasm calc",
		Runtime:     config.RuntimeWASM,
		WASM:        wasm,
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	client, err := reg.Get("calculator")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	res, err := client.CallTool(context.Background(), "add", map[string]any{"a": 10, "b": 32})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.Content[0].Text != "42" {
		t.Fatalf("got %q", res.Content[0].Text)
	}

	infos := reg.List()
	if len(infos) != 1 || infos[0].Runtime != config.RuntimeWASM {
		t.Fatalf("list=%+v", infos)
	}
}

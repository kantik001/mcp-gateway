package mcp_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kantik001/mcp-gateway/internal/mcp"
)

func calculatorWASM(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "calculator.wasm")
}

func TestWASMTransportAddMul(t *testing.T) {
	ctx := context.Background()
	tr, err := mcp.NewWASMTransport(ctx, mcp.WASMConfig{
		Path: calculatorWASM(t),
		Name: "calculator",
	})
	if err != nil {
		t.Fatalf("NewWASMTransport: %v", err)
	}
	defer tr.Close()

	client := mcp.NewClient("calculator", tr)
	if _, err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Tools) != 2 {
		t.Fatalf("tools=%d", len(tools.Tools))
	}

	add, err := client.CallTool(ctx, "add", map[string]any{"a": 2, "b": 3})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if add.IsError || len(add.Content) == 0 || add.Content[0].Text != "5" {
		t.Fatalf("add result=%+v", add)
	}

	mul, err := client.CallTool(ctx, "mul", map[string]any{"a": 4, "b": 5})
	if err != nil {
		t.Fatalf("mul: %v", err)
	}
	if mul.IsError || mul.Content[0].Text != "20" {
		t.Fatalf("mul result=%+v", mul)
	}
}

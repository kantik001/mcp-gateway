package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// WASMConfig configures an in-process WASM MCP transport (wazero host).
type WASMConfig struct {
	// Path to a .wasm module exporting C ABI tools (add/mul/ping).
	Path   string
	Name   string // logical MCP server name
	Logger *slog.Logger
}

// WASMTransport implements Transport by mapping MCP JSON-RPC to sandboxed WASM exports.
// Guests have no filesystem or network capability beyond what WASI preview1 grants
// (we intentionally do not mount preopens).
type WASMTransport struct {
	mu       sync.Mutex
	rt       wazero.Runtime
	mod      api.Module
	name     string
	logger   *slog.Logger
	closed   bool
}

// NewWASMTransport compiles and instantiates the guest module.
func NewWASMTransport(ctx context.Context, cfg WASMConfig) (*WASMTransport, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("wasm path is required")
	}
	abs, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("wasm path: %w", err)
	}
	bin, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read wasm %s: %w", abs, err)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	name := cfg.Name
	if name == "" {
		name = "wasm"
	}

	rt := wazero.NewRuntime(ctx)
	// Provide WASI for guests that import it; calculator itself needs no host calls.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("wasi: %w", err)
	}

	modCfg := wazero.NewModuleConfig().
		WithName(name).
		WithStartFunctions() // do not auto-run _start; we call exports explicitly

	mod, err := rt.InstantiateWithConfig(ctx, bin, modCfg)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("instantiate wasm: %w", err)
	}

	// Require ping export as a smoke check.
	if fn := mod.ExportedFunction("ping"); fn != nil {
		results, err := fn.Call(ctx)
		if err != nil || len(results) == 0 || results[0] != 1 {
			_ = mod.Close(ctx)
			_ = rt.Close(ctx)
			return nil, fmt.Errorf("wasm ping failed: err=%v results=%v", err, results)
		}
	}

	logger.Info("wasm mcp transport ready", "name", name, "path", abs, "bytes", len(bin))
	return &WASMTransport{rt: rt, mod: mod, name: name, logger: logger}, nil
}

// Send handles initialize / tools/list / tools/call against the guest.
func (t *WASMTransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil, fmt.Errorf("wasm transport closed")
	}

	switch method {
	case "initialize":
		res := InitializeResult{
			ProtocolVersion: ProtocolVersion,
			Capabilities: ServerCapabilities{
				Tools: &ToolsCapability{},
			},
			ServerInfo: Implementation{
				Name:    t.name,
				Version: "0.1.0",
			},
			Instructions: "Sandboxed WASM calculator (add, mul). No filesystem or network.",
		}
		return json.Marshal(res)

	case "tools/list":
		res := ListToolsResult{Tools: calculatorTools()}
		return json.Marshal(res)

	case "tools/call":
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		var req CallToolRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("decode tools/call: %w", err)
		}
		return t.callTool(ctx, req)

	default:
		return nil, fmt.Errorf("unsupported method %q on wasm transport", method)
	}
}

// Notify acknowledges MCP notifications (no-op for WASM guests).
func (t *WASMTransport) Notify(ctx context.Context, method string, params any) error {
	_ = ctx
	_ = method
	_ = params
	return nil
}

// Close releases the wazero runtime.
func (t *WASMTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	ctx := context.Background()
	var first error
	if t.mod != nil {
		if err := t.mod.Close(ctx); err != nil {
			first = err
		}
		t.mod = nil
	}
	if t.rt != nil {
		if err := t.rt.Close(ctx); err != nil && first == nil {
			first = err
		}
		t.rt = nil
	}
	return first
}

func (t *WASMTransport) callTool(ctx context.Context, req CallToolRequest) (json.RawMessage, error) {
	a, b, err := numericArgs(req.Arguments)
	if err != nil {
		return marshalToolError(err.Error())
	}

	var export string
	switch req.Name {
	case "add":
		export = "add"
	case "mul":
		export = "mul"
	default:
		return marshalToolError(fmt.Sprintf("unknown tool %q", req.Name))
	}

	fn := t.mod.ExportedFunction(export)
	if fn == nil {
		return marshalToolError(fmt.Sprintf("guest missing export %q", export))
	}
	results, err := fn.Call(ctx, api.EncodeI32(a), api.EncodeI32(b))
	if err != nil {
		return marshalToolError(err.Error())
	}
	if len(results) == 0 {
		return marshalToolError("empty guest result")
	}
	out := api.DecodeI32(results[0])
	res := CallToolResult{
		Content: []Content{{Type: "text", Text: fmt.Sprintf("%d", out)}},
	}
	return json.Marshal(res)
}

func calculatorTools() []Tool {
	schema := json.RawMessage(`{"type":"object","properties":{"a":{"type":"number"},"b":{"type":"number"}},"required":["a","b"]}`)
	return []Tool{
		{Name: "add", Description: "Saturating add of two integers (WASM sandbox)", InputSchema: schema},
		{Name: "mul", Description: "Saturating multiply of two integers (WASM sandbox)", InputSchema: schema},
	}
}

func numericArgs(args map[string]any) (int32, int32, error) {
	if args == nil {
		return 0, 0, fmt.Errorf("missing arguments")
	}
	a, err := toInt32(args["a"])
	if err != nil {
		return 0, 0, fmt.Errorf("arg a: %w", err)
	}
	b, err := toInt32(args["b"])
	if err != nil {
		return 0, 0, fmt.Errorf("arg b: %w", err)
	}
	return a, b, nil
}

func toInt32(v any) (int32, error) {
	switch n := v.(type) {
	case float64:
		if n > math.MaxInt32 || n < math.MinInt32 || n != math.Trunc(n) {
			return 0, fmt.Errorf("not an int32: %v", n)
		}
		return int32(n), nil
	case int:
		if n > math.MaxInt32 || n < math.MinInt32 {
			return 0, fmt.Errorf("out of int32 range")
		}
		return int32(n), nil
	case int32:
		return n, nil
	case int64:
		if n > math.MaxInt32 || n < math.MinInt32 {
			return 0, fmt.Errorf("out of int32 range")
		}
		return int32(n), nil
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, err
		}
		if i > math.MaxInt32 || i < math.MinInt32 {
			return 0, fmt.Errorf("out of int32 range")
		}
		return int32(i), nil
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}

func marshalToolError(msg string) (json.RawMessage, error) {
	res := CallToolResult{
		Content: []Content{{Type: "text", Text: msg}},
		IsError: true,
	}
	return json.Marshal(res)
}

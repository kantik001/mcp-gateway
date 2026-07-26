package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestStdioTransportWithMockSubprocess launches a tiny helper that speaks JSON-RPC over stdio.
func TestStdioTransportWithMockSubprocess(t *testing.T) {
	helper := buildMockMCPHelper(t)

	tr, err := NewStdioTransport(StdioConfig{
		Command: helper,
	})
	if err != nil {
		t.Fatalf("NewStdioTransport: %v", err)
	}
	defer func() { _ = tr.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	raw, err := tr.Send(ctx, "initialize", map[string]any{"protocolVersion": ProtocolVersion})
	if err != nil {
		t.Fatalf("Send initialize: %v", err)
	}
	var init InitializeResult
	if err := json.Unmarshal(raw, &init); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if init.ServerInfo.Name != "helper" {
		t.Fatalf("got %+v", init)
	}

	if err := tr.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	raw, err = tr.Send(ctx, "tools/list", map[string]any{})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	var tools ListToolsResult
	if err := json.Unmarshal(raw, &tools); err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) == 0 {
		t.Fatal("expected tools")
	}

	if err := tr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func buildMockMCPHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "helper.go")
	bin := filepath.Join(dir, "helper")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	code := `package main
import (
  "bufio"
  "encoding/json"
  "os"
)
func main() {
  sc := bufio.NewScanner(os.Stdin)
  sc.Buffer(make([]byte, 0, 1024), 1<<20)
  for sc.Scan() {
    var req map[string]any
    if err := json.Unmarshal(sc.Bytes(), &req); err != nil { continue }
    if _, ok := req["id"]; !ok { continue }
    method, _ := req["method"].(string)
    id := req["id"]
    var result any
    switch method {
    case "initialize":
      result = map[string]any{
        "protocolVersion": "2024-11-05",
        "capabilities": map[string]any{"tools": map[string]any{}},
        "serverInfo": map[string]any{"name":"helper","version":"0.0.1"},
      }
    case "tools/list":
      result = map[string]any{"tools": []map[string]any{{"name":"ping","description":"ping"}}}
    default:
      result = map[string]any{}
    }
    _ = json.NewEncoder(os.Stdout).Encode(map[string]any{"jsonrpc":"2.0","id":id,"result":result})
  }
}
`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}
	return bin
}

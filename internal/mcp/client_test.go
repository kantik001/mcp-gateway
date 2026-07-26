package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestClientInitializeListAndCall(t *testing.T) {
	tr := NewMockTransport()
	client := NewClient("mock", tr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	init, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if init.ServerInfo.Name != "mock" {
		t.Fatalf("unexpected server name: %s", init.ServerInfo.Name)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %+v", tools.Tools)
	}

	result, err := client.CallTool(ctx, "echo", map[string]any{"msg": "hi"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(result.Content) == 0 || result.Content[0].Text == "" {
		t.Fatalf("empty result: %+v", result)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !tr.Closed {
		t.Fatal("expected transport closed")
	}

	methods := map[string]bool{}
	for _, c := range tr.Calls {
		methods[c.Method] = true
	}
	for _, want := range []string{"initialize", "notifications/initialized", "tools/list", "tools/call"} {
		if !methods[want] {
			t.Fatalf("missing call %s in %+v", want, tr.Calls)
		}
	}
}

func TestMockTransportUnknownMethod(t *testing.T) {
	tr := NewMockTransport()
	_, err := tr.Send(context.Background(), "nope", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCallToolResultJSON(t *testing.T) {
	raw := []byte(`{"content":[{"type":"text","text":"ok"}],"isError":false}`)
	var result CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.IsError || result.Content[0].Text != "ok" {
		t.Fatalf("%+v", result)
	}
}

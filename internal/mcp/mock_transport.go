package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// MockTransport is an in-memory Transport for unit tests.
type MockTransport struct {
	mu       sync.Mutex
	Handlers map[string]func(params any) (json.RawMessage, error)
	Closed   bool
	Calls    []MockCall
}

// MockCall records a Send invocation.
type MockCall struct {
	Method string
	Params any
}

// NewMockTransport creates a mock with default initialize/list/call handlers.
func NewMockTransport() *MockTransport {
	m := &MockTransport{
		Handlers: make(map[string]func(params any) (json.RawMessage, error)),
	}
	m.Handlers["initialize"] = func(params any) (json.RawMessage, error) {
		return json.Marshal(InitializeResult{
			ProtocolVersion: ProtocolVersion,
			Capabilities: ServerCapabilities{
				Tools: &ToolsCapability{},
			},
			ServerInfo: Implementation{Name: "mock", Version: "1.0.0"},
		})
	}
	m.Handlers["tools/list"] = func(params any) (json.RawMessage, error) {
		return json.Marshal(ListToolsResult{
			Tools: []Tool{
				{Name: "echo", Description: "Echo arguments as text"},
			},
		})
	}
	m.Handlers["tools/call"] = func(params any) (json.RawMessage, error) {
		req, ok := params.(CallToolRequest)
		if !ok {
			// params may arrive as map from JSON-like usage
			b, _ := json.Marshal(params)
			_ = json.Unmarshal(b, &req)
		}
		msg := fmt.Sprintf("echo:%v", req.Arguments)
		return json.Marshal(CallToolResult{
			Content: []Content{{Type: "text", Text: msg}},
		})
	}
	return m
}

// Send implements Transport.
func (m *MockTransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Closed {
		return nil, fmt.Errorf("transport closed")
	}
	m.Calls = append(m.Calls, MockCall{Method: method, Params: params})
	h, ok := m.Handlers[method]
	if !ok {
		return nil, fmt.Errorf("no handler for %s", method)
	}
	return h(params)
}

// Notify implements Transport.
func (m *MockTransport) Notify(ctx context.Context, method string, params any) error {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Closed {
		return fmt.Errorf("transport closed")
	}
	m.Calls = append(m.Calls, MockCall{Method: method, Params: params})
	return nil
}

// Close implements Transport.
func (m *MockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Closed = true
	return nil
}

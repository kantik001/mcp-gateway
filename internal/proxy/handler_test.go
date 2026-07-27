package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kantik001/mcp-gateway/internal/config"
	"github.com/kantik001/mcp-gateway/internal/mcp"
	"github.com/kantik001/mcp-gateway/internal/proxy"
	"github.com/kantik001/mcp-gateway/internal/registry"
)

// stubRegistry implements registry.Registry for HTTP tests.
type stubRegistry struct {
	client *mcp.Client
}

func (s *stubRegistry) Register(server config.ServerConfig) error { return nil }
func (s *stubRegistry) Get(name string) (*mcp.Client, error) {
	if name != "mock" {
		return nil, errNotFound
	}
	return s.client, nil
}
func (s *stubRegistry) List() []registry.ServerInfo {
	return []registry.ServerInfo{{Name: "mock", Healthy: true, Enabled: true}}
}
func (s *stubRegistry) HealthCheck(ctx context.Context) map[string]bool {
	return map[string]bool{"mock": true}
}
func (s *stubRegistry) Close() error { return nil }

type notFoundError string

func (e notFoundError) Error() string { return string(e) }

var errNotFound = notFoundError(`server "unknown" not found`)

func TestHTTPAPI(t *testing.T) {
	tr := mcp.NewMockTransport()
	client := mcp.NewClient("mock", tr)
	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}

	h := proxy.New(&stubRegistry{client: client}, nil, "", 0)
	srv := h.Routes()

	t.Run("health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
	})

	t.Run("list servers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/servers", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		servers, ok := body["servers"].([]any)
		if !ok || len(servers) != 1 {
			t.Fatalf("body=%v", body)
		}
	})

	t.Run("list tools", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/servers/mock/tools", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("tools schema", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/tools/schema", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			Tools []map[string]any `json:"tools"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Tools) == 0 {
			t.Fatal("expected at least one tool in schema")
		}
		if body.Tools[0]["type"] != "function" {
			t.Fatalf("type=%v", body.Tools[0]["type"])
		}
	})

	t.Run("call tool", func(t *testing.T) {
		payload := []byte(`{"args":{"msg":"hello"}}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/servers/mock/tools/echo", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var result mcp.CallToolResult
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Content) == 0 {
			t.Fatal("empty content")
		}
	})

	t.Run("metrics", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		body := rec.Body.String()
		if !contains(body, "mcp_tool_cost_total") && !contains(body, "mcp_tool_calls_total") && !contains(body, "go_") {
			t.Fatalf("unexpected metrics body: %s", body[:min(200, len(body))])
		}
	})
}

func TestToolCallTimeout(t *testing.T) {
	client := mcp.NewClient("mock", &hangTransport{})
	h := proxy.New(&stubRegistry{client: client}, nil, "", 50*time.Millisecond)
	srv := h.Routes()

	req := httptest.NewRequest(http.MethodPost, "/v1/servers/mock/tools/echo", bytes.NewReader([]byte(`{"args":{}}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// hangTransport blocks until the request context is canceled.
type hangTransport struct{}

func (h *hangTransport) Send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (h *hangTransport) Notify(ctx context.Context, method string, params any) error {
	return nil
}

func (h *hangTransport) Close() error { return nil }

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		bytes.Contains([]byte(s), []byte(sub)))
}

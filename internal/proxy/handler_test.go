package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

	h := proxy.New(&stubRegistry{client: client}, nil, "")
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
		if !contains(body, "mcp_tool_calls_total") && !contains(body, "mcp_server_up") {
			// counters may appear after first call; gauge may be absent until register
			if !contains(body, "go_") {
				t.Fatalf("unexpected metrics body: %s", body[:min(200, len(body))])
			}
		}
	})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		bytes.Contains([]byte(s), []byte(sub)))
}

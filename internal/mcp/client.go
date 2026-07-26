package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// Client is a high-level MCP client over a Transport.
type Client struct {
	transport Transport
	name      string
	logger    *slog.Logger
	init      *InitializeResult
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithLogger sets the client logger.
func WithLogger(l *slog.Logger) ClientOption {
	return func(c *Client) {
		c.logger = l
	}
}

// NewClient wraps a transport as an MCP client.
func NewClient(name string, transport Transport, opts ...ClientOption) *Client {
	c := &Client{
		transport: transport,
		name:      name,
		logger:    slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name returns the logical server name.
func (c *Client) Name() string {
	return c.name
}

// Initialize performs the MCP handshake.
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    ClientCapabilities{},
		ClientInfo: Implementation{
			Name:    "mcp-gateway",
			Version: "0.1.0",
		},
	}

	raw, err := c.transport.Send(ctx, "initialize", params)
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode initialize result: %w", err)
	}
	c.init = &result

	if err := c.transport.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return nil, fmt.Errorf("initialized notification: %w", err)
	}

	c.logger.Info("mcp server initialized",
		"name", c.name,
		"protocol", result.ProtocolVersion,
		"server", result.ServerInfo.Name,
		"version", result.ServerInfo.Version,
	)
	return &result, nil
}

// ListTools returns tools advertised by the server.
func (c *Client) ListTools(ctx context.Context) (*ListToolsResult, error) {
	raw, err := c.transport.Send(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	var result ListToolsResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode tools/list: %w", err)
	}
	return &result, nil
}

// CallTool invokes a named tool with arguments.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*CallToolResult, error) {
	params := CallToolRequest{
		Name:      name,
		Arguments: args,
	}
	raw, err := c.transport.Send(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("tools/call %s: %w", name, err)
	}
	var result CallToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode tools/call: %w", err)
	}
	return &result, nil
}

// Ping checks liveness via tools/list (MCP has no mandatory ping in all servers).
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.ListTools(ctx)
	return err
}

// Close closes the underlying transport.
func (c *Client) Close() error {
	return c.transport.Close()
}

// InitResult returns the last initialize result, if any.
func (c *Client) InitResult() *InitializeResult {
	return c.init
}

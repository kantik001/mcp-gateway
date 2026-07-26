package mcp

import (
	"context"
	"encoding/json"
)

// Transport sends JSON-RPC 2.0 requests to an MCP server and returns results.
type Transport interface {
	Send(ctx context.Context, method string, params any) (json.RawMessage, error)
	Notify(ctx context.Context, method string, params any) error
	Close() error
}

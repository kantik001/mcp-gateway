// Package mcp exposes public MCP types for external consumers.
package mcp

import internalmcp "github.com/kantik001/mcp-gateway/internal/mcp"

// Re-export commonly used types.
type (
	Tool           = internalmcp.Tool
	Content        = internalmcp.Content
	CallToolRequest = internalmcp.CallToolRequest
	CallToolResult = internalmcp.CallToolResult
	ListToolsResult = internalmcp.ListToolsResult
)

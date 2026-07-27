package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// ToolCallsTotal counts tool invocations by server, tool, and status.
	ToolCallsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mcp_tool_calls_total",
		Help: "Total number of MCP tool calls proxied by the gateway",
	}, []string{"server", "tool", "status"})

	// ToolCallDuration observes tool call latency in seconds.
	ToolCallDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mcp_tool_call_duration_seconds",
		Help:    "Duration of MCP tool calls in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"server", "tool"})

	// ServerUp is 1 when the MCP server last health check succeeded.
	ServerUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mcp_server_up",
		Help: "Whether an MCP server is healthy (1) or not (0)",
	}, []string{"server"})

	// HTTPRequestsTotal counts HTTP requests.
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mcp_gateway_http_requests_total",
		Help: "Total HTTP requests handled by the gateway",
	}, []string{"method", "path", "status"})

	// ToolCostTotal approximates cumulative "cost" (token estimate) per tool call.
	// MVP: cost ≈ max(1, len(argsJSON)/4 + len(resultJSON)/4). Future: pricing API.
	ToolCostTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mcp_tool_cost_total",
		Help: "Approximate cumulative tool-call cost (token estimate) by server, tool, and tenant",
	}, []string{"server", "tool", "tenant"})
)

// Handler returns the Prometheus HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

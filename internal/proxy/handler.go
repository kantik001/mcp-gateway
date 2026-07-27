package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kantik001/mcp-gateway/internal/metrics"
	"github.com/kantik001/mcp-gateway/internal/otelx"
	"github.com/kantik001/mcp-gateway/internal/registry"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const defaultToolCallTimeout = 30 * time.Second

// Handler serves the HTTP proxy API.
type Handler struct {
	reg             registry.Registry
	logger          *slog.Logger
	apiKey          string
	toolCallTimeout time.Duration
	defaultTenant   string
	tracer          trace.Tracer
}

// New creates a proxy Handler.
func New(reg registry.Registry, logger *slog.Logger, apiKey string, toolCallTimeout time.Duration) *Handler {
	return NewWithOptions(reg, logger, apiKey, toolCallTimeout, "default")
}

// NewWithOptions creates a Handler with an explicit default tenant label.
func NewWithOptions(reg registry.Registry, logger *slog.Logger, apiKey string, toolCallTimeout time.Duration, defaultTenant string) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if toolCallTimeout <= 0 {
		toolCallTimeout = defaultToolCallTimeout
	}
	if defaultTenant == "" {
		defaultTenant = "default"
	}
	return &Handler{
		reg:             reg,
		logger:          logger,
		apiKey:          apiKey,
		toolCallTimeout: toolCallTimeout,
		defaultTenant:   defaultTenant,
		tracer:          otelx.Tracer("mcp-gateway/proxy"),
	}
}

// Routes mounts all HTTP routes on a chi router (wrapped with OpenTelemetry HTTP instrumentation).
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(h.requestLogger)
	r.Use(middleware.Recoverer)
	r.Use(h.optionalAPIKey)

	r.Get("/health", h.health)
	r.Get("/registry", h.listServers) // alias for convenience
	r.Get("/metrics", metrics.Handler().ServeHTTP)

	r.Route("/v1", func(r chi.Router) {
		r.Get("/servers", h.listServers)
		r.Get("/tools/schema", h.toolsSchema)
		r.Get("/servers/{name}/tools", h.listTools)
		r.Post("/servers/{name}/tools/{tool}", h.callTool)
	})

	return otelhttp.NewHandler(r, "mcp-gateway")
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

func (h *Handler) listServers(w http.ResponseWriter, r *http.Request) {
	servers := h.reg.List()
	writeJSON(w, http.StatusOK, map[string]any{
		"servers": servers,
	})
}

func (h *Handler) listTools(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	ctx, span := h.tracer.Start(r.Context(), "mcp.tools.list",
		trace.WithAttributes(attribute.String("mcp.server", name)))
	defer span.End()

	client, err := h.reg.Get(name)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(ctx, h.toolCallTimeout)
	defer cancel()

	ctx, rpcSpan := h.tracer.Start(ctx, "mcp.jsonrpc",
		trace.WithAttributes(attribute.String("rpc.method", "tools/list")))
	result, err := client.ListTools(ctx)
	rpcSpan.End()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		writeError(w, upstreamStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// OpenAIFunctionTool is one tool in OpenAI function-calling shape (plus MCP metadata).
type OpenAIFunctionTool struct {
	Type     string             `json:"type"`
	Function OpenAIFunctionBody `json:"function"`
	Server   string             `json:"server"`
	MCPTool  string             `json:"mcp_tool"`
}

// OpenAIFunctionBody is the nested function object.
type OpenAIFunctionBody struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

func (h *Handler) toolsSchema(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tracer.Start(r.Context(), "mcp.tools.schema")
	defer span.End()

	servers := h.reg.List()
	out := make([]OpenAIFunctionTool, 0)
	for _, info := range servers {
		client, err := h.reg.Get(info.Name)
		if err != nil {
			h.logger.Warn("tools schema skip server", "server", info.Name, "error", err)
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, h.toolCallTimeout)
		_, rpcSpan := h.tracer.Start(cctx, "mcp.jsonrpc",
			trace.WithAttributes(
				attribute.String("rpc.method", "tools/list"),
				attribute.String("mcp.server", info.Name),
			))
		result, err := client.ListTools(cctx)
		rpcSpan.End()
		cancel()
		if err != nil {
			h.logger.Warn("tools schema list failed", "server", info.Name, "error", err)
			continue
		}
		for _, tool := range result.Tools {
			params := tool.InputSchema
			if len(params) == 0 {
				params = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			out = append(out, OpenAIFunctionTool{
				Type: "function",
				Function: OpenAIFunctionBody{
					Name:        info.Name + "." + tool.Name,
					Description: tool.Description,
					Parameters:  params,
				},
				Server:  info.Name,
				MCPTool: tool.Name,
			})
		}
	}
	span.SetAttributes(attribute.Int("tools.count", len(out)))
	writeJSON(w, http.StatusOK, map[string]any{"tools": out})
}

type callToolBody struct {
	Args map[string]any `json:"args"`
}

func (h *Handler) callTool(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	tool := chi.URLParam(r, "tool")
	tenant := tenantFromRequest(r, h.defaultTenant)

	ctx, span := h.tracer.Start(r.Context(), "mcp.tools.call",
		trace.WithAttributes(
			attribute.String("mcp.server", name),
			attribute.String("mcp.tool", tool),
			attribute.String("tenant", tenant),
		))
	defer span.End()

	client, err := h.reg.Get(name)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var body callToolBody
	if r.Body != nil && r.ContentLength != 0 {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}
	if body.Args == nil {
		body.Args = map[string]any{}
	}

	ctx, cancel := context.WithTimeout(ctx, h.toolCallTimeout)
	defer cancel()

	start := time.Now()
	ctx, rpcSpan := h.tracer.Start(ctx, "mcp.jsonrpc",
		trace.WithAttributes(attribute.String("rpc.method", "tools/call")))
	result, err := client.CallTool(ctx, tool, body.Args)
	rpcSpan.End()
	metrics.ToolCallDuration.WithLabelValues(name, tool).Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.ToolCallsTotal.WithLabelValues(name, tool, "error").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		writeError(w, upstreamStatus(err), err.Error())
		return
	}
	status := "ok"
	if result.IsError {
		status = "tool_error"
	}
	metrics.ToolCallsTotal.WithLabelValues(name, tool, status).Inc()

	argsJSON, _ := json.Marshal(body.Args)
	resultJSON, _ := json.Marshal(result)
	cost := estimateTokenCost(argsJSON, resultJSON)
	metrics.ToolCostTotal.WithLabelValues(name, tool, tenant).Add(float64(cost))
	span.SetAttributes(attribute.Int("mcp.cost_tokens_est", cost))

	writeJSON(w, http.StatusOK, result)
}

func estimateTokenCost(argsJSON, resultJSON []byte) int {
	n := (len(argsJSON) + len(resultJSON)) / 4
	if n < 1 {
		return 1
	}
	return n
}

func tenantFromRequest(r *http.Request, fallback string) string {
	if t := r.Header.Get("X-Tenant-ID"); t != "" {
		return t
	}
	if t := r.URL.Query().Get("tenant"); t != "" {
		return t
	}
	return fallback
}

func upstreamStatus(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout
	}
	if errors.Is(err, context.Canceled) {
		return http.StatusRequestTimeout
	}
	return http.StatusBadGateway
}

func (h *Handler) optionalAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.apiKey == "" {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		key := r.Header.Get("X-API-Key")
		if key == "" {
			auth := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if len(auth) > len(prefix) && auth[:len(prefix)] == prefix {
				key = auth[len(prefix):]
			}
		}
		if key != h.apiKey {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		reqID := middleware.GetReqID(r.Context())
		path := r.URL.Path
		span := trace.SpanFromContext(r.Context())
		if span.SpanContext().HasTraceID() {
			span.SetAttributes(attribute.String("request_id", reqID))
		}
		h.logger.Info("request",
			"request_id", reqID,
			"trace_id", span.SpanContext().TraceID().String(),
			"method", r.Method,
			"path", path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		)
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, http.StatusText(ww.Status())).Inc()
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": msg,
	})
}

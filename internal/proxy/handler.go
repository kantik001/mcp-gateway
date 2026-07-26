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
	"github.com/kantik001/mcp-gateway/internal/registry"
)

const defaultToolCallTimeout = 30 * time.Second

// Handler serves the HTTP proxy API.
type Handler struct {
	reg             registry.Registry
	logger          *slog.Logger
	apiKey          string
	toolCallTimeout time.Duration
}

// New creates a proxy Handler.
func New(reg registry.Registry, logger *slog.Logger, apiKey string, toolCallTimeout time.Duration) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if toolCallTimeout <= 0 {
		toolCallTimeout = defaultToolCallTimeout
	}
	return &Handler{reg: reg, logger: logger, apiKey: apiKey, toolCallTimeout: toolCallTimeout}
}

// Routes mounts all HTTP routes on a chi router.
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
		r.Get("/servers/{name}/tools", h.listTools)
		r.Post("/servers/{name}/tools/{tool}", h.callTool)
	})

	return r
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
	client, err := h.reg.Get(name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.toolCallTimeout)
	defer cancel()
	result, err := client.ListTools(ctx)
	if err != nil {
		writeError(w, upstreamStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type callToolBody struct {
	Args map[string]any `json:"args"`
}

func (h *Handler) callTool(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	tool := chi.URLParam(r, "tool")

	client, err := h.reg.Get(name)
	if err != nil {
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

	ctx, cancel := context.WithTimeout(r.Context(), h.toolCallTimeout)
	defer cancel()

	start := time.Now()
	result, err := client.CallTool(ctx, tool, body.Args)
	metrics.ToolCallDuration.WithLabelValues(name, tool).Observe(time.Since(start).Seconds())
	if err != nil {
		metrics.ToolCallsTotal.WithLabelValues(name, tool, "error").Inc()
		writeError(w, upstreamStatus(err), err.Error())
		return
	}
	// MCP tool-level failures (isError: true) are successful JSON-RPC responses.
	// Keep HTTP 200 so agents can read content + isError without treating it as a transport fault.
	status := "ok"
	if result.IsError {
		status = "tool_error"
	}
	metrics.ToolCallsTotal.WithLabelValues(name, tool, status).Inc()
	writeJSON(w, http.StatusOK, result)
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
		// Allow unauthenticated health and metrics for orchestration probes.
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
		h.logger.Info("request",
			"request_id", reqID,
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

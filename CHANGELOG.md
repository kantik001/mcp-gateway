# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Documentation aligned with Go **1.25**, Node **22** runtime image, env vars (`OTEL_SERVICE_NAME`, `OTEL_SDK_DISABLED`), and current API/metrics behavior
- `OTEL_SERVICE_NAME` is honored (default `mcp-gateway`) for Jaeger / OTLP resource naming

### Added

- **OpenTelemetry** distributed tracing (OTLP/HTTP → Jaeger); spans for HTTP → `mcp.tools.*` → `mcp.jsonrpc`
- Jaeger all-in-one in `docker-compose.yml` (UI `:16686`)
- `GET /v1/tools/schema` — OpenAI function-calling JSON for agent tool discovery
- Prometheus `mcp_tool_cost_total{server,tool,tenant}` (MVP token estimate)
- **gRPC** `grpc.health.v1.Health/Check` on `GRPC_PORT` (default `:8081`)

### MVP (baseline)

- Stdio MCP client (JSON-RPC 2.0): initialize, tools/list, tools/call
- In-memory registry with periodic health checks
- HTTP API: `/health`, `/v1/servers`, tools list/call, `/metrics`
- Docker Compose stack (gateway + Postgres + Redis reserved)
- GitHub Actions CI (test, coverage gate, lint, build)
- Optional `API_KEY` authentication for `/v1/*`
- Per-call MCP timeout via `TOOL_CALL_TIMEOUT` (default `30s`); timeouts return HTTP 504

### Removed

- Go Report Card badge (service sunset July 2026; quality checks covered by golangci-lint in CI)

[Unreleased]: https://github.com/kantik001/mcp-gateway/commits/main

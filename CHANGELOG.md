# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial MVP release
- Stdio MCP client (JSON-RPC 2.0): initialize, tools/list, tools/call
- In-memory registry with periodic health checks
- HTTP API: `/health`, `/v1/servers`, tools list/call, `/metrics`
- Docker Compose stack (gateway + Postgres + Redis reserved)
- GitHub Actions CI (test, coverage gate, lint, build)
- Optional `API_KEY` authentication for `/v1/*`

[Unreleased]: https://github.com/kantik001/mcp-gateway/compare/v0.1.0...HEAD

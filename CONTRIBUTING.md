# Contributing to mcp-gateway

Thanks for helping improve the gateway.

## Development setup

1. Install Go **1.25+**
2. Clone the repo and copy env defaults:

   ```bash
   cp .env.example .env
   make tidy
   make test
   ```

3. Optional: install [golangci-lint](https://golangci-lint.run/) **v2** for `make lint` (CI uses v2.12.x)

## Guidelines

- Keep the MVP focused: HTTP proxy + stdio MCP + in-memory registry
- Prefer small, tested changes over large refactors
- Add unit tests for `internal/mcp` and HTTP handlers when behavior changes
- Use structured logging (`log/slog`); include `request_id` on HTTP paths
- Do not commit secrets (`.env`, API keys, credentials)

## Pull requests

1. Fork and create a feature branch
2. Ensure `go test ./...` passes
3. Open a PR with a short summary and test notes

## Code of conduct

Be respectful. Harassment or abuse is not tolerated.

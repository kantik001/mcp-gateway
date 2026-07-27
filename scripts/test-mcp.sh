#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
GRPC_ADDR="${GRPC_ADDR:-localhost:8081}"

echo "==> Health"
curl -fsS "$BASE_URL/health" | tee /tmp/mcp-health.json
echo

echo "==> List servers"
curl -fsS "$BASE_URL/v1/servers" | tee /tmp/mcp-servers.json
echo

echo "==> Tool schema (OpenAI function-calling shape)"
curl -fsS "$BASE_URL/v1/tools/schema" | tee /tmp/mcp-schema.json | head -c 400
echo
echo

echo "==> List filesystem tools"
curl -fsS "$BASE_URL/v1/servers/filesystem/tools" | tee /tmp/mcp-tools.json
echo

echo "==> Read /data/README.md via filesystem MCP"
curl -fsS -X POST "$BASE_URL/v1/servers/filesystem/tools/read_file" \
  -H "Content-Type: application/json" \
  -d '{"args":{"path":"/data/README.md"}}' | tee /tmp/mcp-read.json
echo

echo "==> Prometheus metrics (sample)"
curl -fsS "$BASE_URL/metrics" | grep -E 'mcp_tool_calls_total|mcp_tool_cost_total|mcp_server_up' || true
echo

if command -v grpcurl >/dev/null 2>&1; then
  echo "==> gRPC health"
  grpcurl -plaintext "$GRPC_ADDR" grpc.health.v1.Health/Check
  echo
else
  echo "==> gRPC health skipped (install grpcurl to enable)"
fi

echo "OK"

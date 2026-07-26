#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"

echo "==> Health"
curl -fsS "$BASE_URL/health" | tee /tmp/mcp-health.json
echo

echo "==> List servers"
curl -fsS "$BASE_URL/v1/servers" | tee /tmp/mcp-servers.json
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
curl -fsS "$BASE_URL/metrics" | grep -E 'mcp_tool_calls_total|mcp_server_up' || true
echo
echo "OK"

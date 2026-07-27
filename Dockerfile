# Build stage
FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/mcp-gateway ./cmd/server

# Runtime stage — needs Node for npx-based MCP servers
FROM node:22-bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    python3 \
    python3-pip \
    python3-venv \
    && rm -rf /var/lib/apt/lists/* \
    && pip3 install --break-system-packages uv \
    && useradd --create-home --uid 10001 gateway

WORKDIR /app

COPY --from=builder /out/mcp-gateway /app/mcp-gateway
COPY config/servers.yaml /app/config/servers.yaml
COPY wasm/calculator.wasm /app/wasm/calculator.wasm
COPY docker/data /data

RUN chown -R gateway:gateway /app /data

USER gateway
ENV PORT=8080 \
    GRPC_PORT=8081 \
    CONFIG_PATH=/app/config/servers.yaml \
    LOG_LEVEL=info \
    HOME=/home/gateway

EXPOSE 8080 8081
HEALTHCHECK --interval=15s --timeout=5s --start-period=40s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/mcp-gateway"]

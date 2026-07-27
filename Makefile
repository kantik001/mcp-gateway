.PHONY: build test run lint docker-up docker-down tidy coverage wasm-calculator

APP_NAME=mcp-gateway
WASM_GUEST=wasm/guests/calculator
WASM_OUT=wasm/calculator.wasm

build:
	go build -o bin/$(APP_NAME)$(shell go env GOEXE) ./cmd/server

test:
	go test ./...

coverage:
	go test ./internal/mcp/... -coverprofile=coverage.out
	go tool cover -func=coverage.out

run: build
	./bin/$(APP_NAME)$(shell go env GOEXE)

lint:
	golangci-lint run ./...

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

tidy:
	go mod tidy

# Requires: rustup target add wasm32-unknown-unknown
wasm-calculator:
	cd $(WASM_GUEST) && cargo build --release --target wasm32-unknown-unknown
	cp $(WASM_GUEST)/target/wasm32-unknown-unknown/release/calculator.wasm $(WASM_OUT)
	cp $(WASM_OUT) internal/mcp/testdata/calculator.wasm

.PHONY: build test run lint docker-up docker-down tidy coverage

APP_NAME=mcp-gateway

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

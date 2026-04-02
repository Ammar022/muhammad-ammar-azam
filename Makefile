# ============================================================
# Makefile for secure-ai-chat-backend
# ============================================================

BINARY_NAME    := secure-ai-chat
BUILD_DIR      := ./bin
CMD_PATH       := ./cmd/api
MIGRATION_DIR  := ./migrations
DB_URL          = postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)

# Load .env if present (for local convenience)
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

.PHONY: all build run clean test test-unit test-integration format tidy cover \
        migrate-up migrate-down docker-up docker-down docker-db

## all: Build the binary
all: build

## build: Compile the Go binary
build:
	@echo ">> Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 go build -ldflags="-w -s" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@echo ">> Binary built at $(BUILD_DIR)/$(BINARY_NAME)"

## run: Run the application locally (requires .env)
run:
	@echo ">> Starting $(BINARY_NAME)..."
	@go run $(CMD_PATH)/main.go

## format: Format code, run vet, and lint
format:
	@echo ">> Formatting code..."
	@gofmt -w .
	@echo ">> Running go vet..."
	@go vet ./...
	@echo ">> Running linter..."
	@golangci-lint run ./...


## test: Run all tests
test:
	@echo ">> Running all tests..."
	@go test -v -race -count=1 ./...

## test-unit: Run unit tests only
test-unit:
	@echo ">> Running unit tests..."
	@go test -v -race -count=1 ./tests/unit/...

## test-integration: Run integration tests only
test-integration:
	@echo ">> Running integration tests..."
	@go test -v -race -count=1 ./tests/integration/...

## cover: Run tests with coverage report
cover:
	@echo ">> Running tests with coverage..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo ">> Coverage report written to coverage.html"

## migrate-up: Apply all pending database migrations
migrate-up:
	@echo ">> Applying database migrations..."
	@migrate -path $(MIGRATION_DIR) -database "$(DB_URL)" up

## migrate-down: Roll back the last migration
migrate-down:
	@echo ">> Rolling back last migration..."
	@migrate -path $(MIGRATION_DIR) -database "$(DB_URL)" down 1

## docker-up: Start all Docker services (PostgreSQL + API)
docker-up:
	@echo ">> Starting Docker services..."
	@docker-compose up -d

## docker-db: Start only the PostgreSQL container (then use 'make run' for the API)
docker-db:
	@echo ">> Starting PostgreSQL only..."
	@docker-compose up -d postgres

## docker-down: Stop all Docker services
docker-down:
	@echo ">> Stopping Docker services..."
	@docker-compose down

## help: Display this help
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

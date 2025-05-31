# Pump Fun Bot Go - Makefile

# Variables
BINARY_NAME=pump-fun-bot
BUILD_DIR=build
GO_FILES=$(shell find . -name "*.go" -type f)
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Default target
.PHONY: all
all: clean build

# Build the application
.PHONY: build
build:
	@echo "Building $(BINARY_NAME) version $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) cmd/bot/main.go

# Build for multiple platforms
.PHONY: build-all
build-all: clean
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)

	# Linux AMD64
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 cmd/bot/main.go

	# Linux ARM64
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 cmd/bot/main.go

	# macOS AMD64
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 cmd/bot/main.go

	# macOS ARM64 (Apple Silicon)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 cmd/bot/main.go

	# Windows AMD64
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe cmd/bot/main.go

# Run the application
.PHONY: run
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# Run with YOLO mode
.PHONY: run-yolo
run-yolo: build
	@echo "Running $(BINARY_NAME) in YOLO mode..."
	./$(BUILD_DIR)/$(BINARY_NAME) --yolo

# Run with hold mode
.PHONY: run-hold
run-hold: build
	@echo "Running $(BINARY_NAME) in hold mode..."
	./$(BUILD_DIR)/$(BINARY_NAME) --hold

# Run with devnet
.PHONY: run-devnet
run-devnet: build
	@echo "Running $(BINARY_NAME) on devnet..."
	./$(BUILD_DIR)/$(BINARY_NAME) --network devnet

# Run with debug logging
.PHONY: run-debug
run-debug: build
	@echo "Running $(BINARY_NAME) with debug logging..."
	./$(BUILD_DIR)/$(BINARY_NAME) --log-level debug

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmarks
.PHONY: bench
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# Lint the code
.PHONY: lint
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Format the code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	else \
		echo "goimports not installed. Install with: go install golang.org/x/tools/cmd/goimports@latest"; \
	fi

# Tidy dependencies
.PHONY: tidy
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

# Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	go mod download

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Install development tools
.PHONY: install-tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/air-verse/air@latest

# Setup development environment
.PHONY: setup
setup: install-tools deps
	@echo "Setting up development environment..."
	@mkdir -p logs trades configs/strategies configs/idl
	@echo "Development environment setup complete!"

# Watch and reload (requires air)
.PHONY: dev
dev:
	@echo "Starting development server with hot reload..."
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "air not installed. Install with: make install-tools"; \
	fi

# Docker targets
.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	docker build -t pump-fun-bot:$(VERSION) .

.PHONY: docker-run
docker-run:
	@echo "Running Docker container..."
	docker run --rm -v $(PWD)/configs:/app/configs -v $(PWD)/logs:/app/logs -v $(PWD)/trades:/app/trades pump-fun-bot:$(VERSION)

# Quick debugging
.PHONY: debug
debug:
	@chmod +x scripts/quick_debug.sh
	@./scripts/quick_debug.sh

# Debug WebSocket connection
.PHONY: debug-ws
debug-ws:
	@echo "Running WebSocket debug tool..."
	@cd scripts && go run debug_websocket.go

# Debug WebSocket on mainnet (be careful!)
.PHONY: debug-ws-mainnet
debug-ws-mainnet:
	@echo "Running WebSocket debug tool on MAINNET..."
	@cd scripts && go run debug_websocket.go mainnet-ws
debug-ws:
	@echo "Running WebSocket debug tool..."
	@cd scripts && go run debug_websocket.go

# Debug WebSocket on mainnet (be careful!)
.PHONY: debug-ws-mainnet
debug-ws-mainnet:
	@echo "Running WebSocket debug tool on MAINNET..."
	@cd scripts && go run debug_websocket.go mainnet

# Make test script executable
.PHONY: test-script
test-script:
	@chmod +x scripts/test_bot.sh
	@echo "Test script is now executable. Run with: ./scripts/test_bot.sh"

# Run comprehensive tests
.PHONY: test-all
test-all: build test-script
	@echo "Running comprehensive bot tests..."
	@./scripts/test_bot.sh

# Quick test on devnet
.PHONY: test-devnet
test-devnet: build
	@echo "Quick devnet test (requires PUMPBOT_PRIVATE_KEY)..."
	@timeout 10 ./build/pump-fun-bot --network devnet --log-level info --dry-run || true
	@echo "Devnet test completed"

# Help
.PHONY: help
help:
	@echo "Available commands:"
	@echo "  build        - Build the application"
	@echo "  build-all    - Build for multiple platforms"
	@echo "  run          - Build and run the application"
	@echo "  run-yolo     - Run in YOLO mode (continuous trading)"
	@echo "  run-hold     - Run in hold mode (no selling)"
	@echo "  run-devnet   - Run on devnet"
	@echo "  run-debug    - Run with debug logging"
	@echo "  test         - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  test-all     - Run comprehensive bot tests"
	@echo "  test-devnet  - Quick devnet connection test"
	@echo "  bench        - Run benchmarks"
	@echo "  lint         - Run linter"
	@echo "  fmt          - Format code"
	@echo "  tidy         - Tidy dependencies"
	@echo "  deps         - Download dependencies"
	@echo "  clean        - Clean build artifacts"
	@echo "  install-tools - Install development tools"
	@echo "  setup        - Setup development environment"
	@echo "  dev          - Start development server with hot reload"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run Docker container"
	@echo "  help         - Show this help message"pump-fun-bot --network devnet --log-level info --dry-run &
	@sleep 10
	@pkill pump-fun-bot || true
	@echo "Devnet test completed"

# Example usage targets
.PHONY: examples
examples:
	@echo "Example usage:"
	@echo ""
	@echo "# Basic usage:"
	@echo "make run"
	@echo ""
	@echo "# YOLO mode (continuous trading):"
	@echo "make run-yolo"
	@echo "# OR: ./build/pump-fun-bot --yolo"
	@echo ""
	@echo "# Filter by token name:"
	@echo "./build/pump-fun-bot --match 'doge'"
	@echo ""
	@echo "# Filter by creator:"
	@echo "./build/pump-fun-bot --creator '7YmjpX4sPPw9pq6P2hrq9LehAi6QjELPWZYKXRrLaLCB'"
	@echo ""
	@echo "# Hold only mode:"
	@echo "./build/pump-fun-bot --hold"
	@echo ""
	@echo "# Combined filters:"
	@echo "./build/pump-fun-bot --match 'pepe' --creator '7Ymj...' --hold"
	@echo ""
	@echo "# Using environment variables:"
	@echo "export PUMPBOT_PRIVATE_KEY='your_private_key_here'"
	@echo "export PUMPBOT_TRADING_BUY_AMOUNT_SOL=0.05"
	@echo "make run"
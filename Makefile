# Pump Fun Bot Go - Ultra-Fast Mode Only

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

# Run the application (ultra-fast mode)
.PHONY: run
run: build
	@echo "Running $(BINARY_NAME) in ultra-fast mode..."
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

# 🚀 Quick start commands for different modes
.PHONY: quick-start
quick-start:
	@echo "🚀 Quick Start Commands (Ultra-Fast Mode):"
	@echo ""
	@echo "📋 Basic Commands:"
	@echo "make run-devnet              # Safe devnet testing"
	@echo "make run-debug               # Debug mode"
	@echo ""
	@echo "⚡ Performance Commands:"
	@echo "make run-hold                # Hold-only mode"
	@echo "make run-yolo                # YOLO mode (dangerous!)"
	@echo ""
	@echo "🔧 Development Commands:"
	@echo "make test-all                # Run all tests"
	@echo "make debug                   # Quick debugging"

# Help with ultra-fast specific examples
.PHONY: help-ultra-fast
help-ultra-fast:
	@echo "⚡⚡ Ultra-Fast Mode Commands:"
	@echo ""
	@echo "Basic ultra-fast usage:"
	@echo "  make run-devnet                             # Safe testing on devnet"
	@echo "  make run                                    # Live trading (ultra-fast)"
	@echo ""
	@echo "Advanced ultra-fast usage:"
	@echo "  ./build/pump-fun-bot --parallel-workers 5          # 5 parallel workers"
	@echo "  ./build/pump-fun-bot --skip-validation             # Maximum speed"
	@echo "  ./build/pump-fun-bot --benchmark --log-latency     # Performance testing"
	@echo ""
	@echo "Environment variable examples:"
	@echo "  PUMPBOT_ULTRA_FAST_PARALLEL_WORKERS=5 make run"
	@echo "  PUMPBOT_ULTRA_FAST_SKIP_VALIDATION=true make run"
	@echo ""
	@echo "💡 Tips:"
	@echo "  - Always test with --dry-run first"
	@echo "  - Start with 3 parallel workers"
	@echo "  - Use latency logging for optimization"
	@echo "  - Monitor success rates vs speed"

# Help
.PHONY: help
help:
	@echo "Available commands (Ultra-Fast Mode Only):"
	@echo ""
	@echo "🏗️  Build & Run:"
	@echo "  build        - Build the application"
	@echo "  build-all    - Build for multiple platforms"
	@echo "  run          - Build and run in ultra-fast mode"
	@echo "  run-devnet   - Run on devnet"
	@echo "  run-debug    - Run with debug logging"
	@echo ""
	@echo "⚡ Trading Modes:"
	@echo "  run-yolo     - Run in YOLO mode (continuous trading)"
	@echo "  run-hold     - Run in hold mode (no selling)"
	@echo ""
	@echo "🧪 Testing:"
	@echo "  test         - Run unit tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  test-all     - Run comprehensive bot tests"
	@echo "  test-devnet  - Quick devnet connection test"
	@echo "  bench        - Run benchmarks"
	@echo ""
	@echo "🔧 Development:"
	@echo "  lint         - Run linter"
	@echo "  fmt          - Format code"
	@echo "  tidy         - Tidy dependencies"
	@echo "  deps         - Download dependencies"
	@echo "  clean        - Clean build artifacts"
	@echo "  setup        - Setup development environment"
	@echo "  dev          - Start development server with hot reload"
	@echo ""
	@echo "🐳 Docker:"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run Docker container"
	@echo ""
	@echo "🚀 Quick Start:"
	@echo "  quick-start     - Show quick start commands"
	@echo "  help-ultra-fast - Show ultra-fast specific help"

# Example usage targets
.PHONY: examples
examples:
	@echo "🚀 Example Usage Commands (Ultra-Fast Mode):"
	@echo ""
	@echo "🔰 Beginner (Safe Testing):"
	@echo "make run-devnet"
	@echo "# OR: ./build/pump-fun-bot --network devnet --dry-run"
	@echo ""
	@echo "⚡ Maximum Speed:"
	@echo "make run --skip-validation --parallel-workers 5"
	@echo "# OR: ./build/pump-fun-bot --skip-validation --parallel-workers 5"
	@echo ""
	@echo "🎯 Filtered Trading:"
	@echo "./build/pump-fun-bot --network mainnet --match 'doge' --hold"
	@echo ""
	@echo "💰 High-Frequency Trading:"
	@echo "./build/pump-fun-bot --network mainnet --yolo --skip-validation --parallel-workers 5"
	@echo ""
	@echo "🔧 Custom Configuration:"
	@echo "PUMPBOT_TRADING_BUY_AMOUNT_SOL=0.05 PUMPBOT_ULTRA_FAST_PARALLEL_WORKERS=3 make run"
	@echo ""
	@echo "📊 Testing & Development:"
	@echo "./build/pump-fun-bot --network devnet --dry-run --log-level debug --benchmark"
	@echo ""
	@echo "⚠️  Always start with devnet and --dry-run for testing!"

# Performance benchmarking
.PHONY: bench-ultra-fast
bench-ultra-fast:
	@echo "🏃‍♂️ Benchmarking ultra-fast performance..."
	@echo "Testing different worker counts and configurations..."
	@echo ""
	@echo "Running bot performance test..."
	@timeout 30 ./build/pump-fun-bot --network devnet --dry-run --log-level info --benchmark || true
	@echo "Ultra-fast performance benchmarking completed"

# Create example environment files
.PHONY: create-env-examples
create-env-examples:
	@echo "Creating example environment files..."
	@mkdir -p configs

	@echo "# Conservative ultra-fast setup for beginners" > configs/.env.conservative
	@echo "PUMPBOT_NETWORK=devnet" >> configs/.env.conservative
	@echo "PUMPBOT_TRADING_BUY_AMOUNT_SOL=0.001" >> configs/.env.conservative
	@echo "PUMPBOT_STRATEGY_HOLD_ONLY=true" >> configs/.env.conservative
	@echo "PUMPBOT_ULTRA_FAST_PARALLEL_WORKERS=1" >> configs/.env.conservative
	@echo "PUMPBOT_LOGGING_LEVEL=info" >> configs/.env.conservative

	@echo "# Maximum speed ultra-fast trading" > configs/.env.max-speed
	@echo "PUMPBOT_NETWORK=mainnet" >> configs/.env.max-speed
	@echo "PUMPBOT_ULTRA_FAST_SKIP_VALIDATION=true" >> configs/.env.max-speed
	@echo "PUMPBOT_ULTRA_FAST_PARALLEL_WORKERS=5" >> configs/.env.max-speed

	@echo "# Aggressive ultra-fast trading (high risk)" > configs/.env.aggressive
	@echo "PUMPBOT_NETWORK=mainnet" >> configs/.env.aggressive
	@echo "PUMPBOT_STRATEGY_YOLO_MODE=true" >> configs/.env.aggressive
	@echo "PUMPBOT_TRADING_BUY_AMOUNT_SOL=0.05" >> configs/.env.aggressive
	@echo "PUMPBOT_ULTRA_FAST_SKIP_VALIDATION=true" >> configs/.env.aggressive
	@echo "PUMPBOT_ULTRA_FAST_PARALLEL_WORKERS=5" >> configs/.env.aggressive
	@echo "PUMPBOT_ULTRA_FAST_PRIORITY_OVER_SAFETY=true" >> configs/.env.aggressive

	@echo "✅ Created example environment files:"
	@echo "  - configs/.env.conservative  (Safe for beginners)"
	@echo "  - configs/.env.max-speed     (Maximum speed)"
	@echo "  - configs/.env.aggressive    (High risk/reward)"
	@echo ""
	@echo "Usage: ./build/pump-fun-bot --env configs/.env.conservative"

# Comprehensive setup for new users
.PHONY: setup-complete
setup-complete: setup create-env-examples
	@echo ""
	@echo "🎉 Complete ultra-fast setup finished!"
	@echo ""
	@echo "📋 Next steps:"
	@echo "1. Copy .env.example to .env and set your PUMPBOT_PRIVATE_KEY"
	@echo "2. Test the setup: make test-devnet"
	@echo "3. Start trading: make run-devnet"
	@echo ""
	@echo "📚 Learn more:"
	@echo "- make help-ultra-fast  # Ultra-fast specific commands"
	@echo "- make examples         # Usage examples"
	@echo "- make quick-start      # Quick start guide"

# Validate current configuration
.PHONY: validate-config
validate-config:
	@echo "🔍 Validating current configuration..."
	@if [ -z "$$PUMPBOT_PRIVATE_KEY" ]; then \
		echo "❌ PUMPBOT_PRIVATE_KEY not set"; \
		echo "Set it with: export PUMPBOT_PRIVATE_KEY='your_key_here'"; \
		exit 1; \
	else \
		echo "✅ Private key is set"; \
	fi

	@echo "Network: $${PUMPBOT_NETWORK:-mainnet}"
	@echo "Buy Amount: $${PUMPBOT_TRADING_BUY_AMOUNT_SOL:-0.01} SOL"
	@echo "Parallel Workers: $${PUMPBOT_ULTRA_FAST_PARALLEL_WORKERS:-3}"
	@echo "Skip Validation: $${PUMPBOT_ULTRA_FAST_SKIP_VALIDATION:-false}"
	@echo ""
	@echo "✅ Configuration validation complete"

# Show current costs estimation
.PHONY: estimate-costs
estimate-costs:
	@echo "💰 Trading Cost Estimation (Ultra-Fast Mode)"
	@echo "============================================"
	@echo ""
	@echo "Based on current configuration:"
	@echo "Buy Amount: $${PUMPBOT_TRADING_BUY_AMOUNT_SOL:-0.01} SOL"
	@echo "Parallel Workers: $${PUMPBOT_ULTRA_FAST_PARALLEL_WORKERS:-3}"
	@echo ""
	@echo "Cost breakdown per trade:"
	@echo "- Base transaction fee: ~0.000005 SOL"
	@echo "- Priority fee: ~0.000010-0.000050 SOL"
	@echo ""
	@echo "Estimated total: 0.000015-0.000055 SOL per trade"
	@echo "As percentage of buy amount: 0.15%-0.55%"
	@echo ""
	@echo "💡 Tips to reduce costs:"
	@echo "- Use lower priority fees during low congestion"
	@echo "- Optimize parallel workers based on your setup"
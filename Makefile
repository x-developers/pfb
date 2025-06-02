# Pump Fun Bot Go - Makefile with Jito Support

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

# 🛡️ NEW: Jito-related targets
.PHONY: run-jito
run-jito: build
	@echo "Running $(BINARY_NAME) with Jito MEV protection..."
	./$(BUILD_DIR)/$(BINARY_NAME) --jito

# Run with Jito on devnet (testing)
.PHONY: run-jito-devnet
run-jito-devnet: build
	@echo "Running $(BINARY_NAME) with Jito on devnet..."
	./$(BUILD_DIR)/$(BINARY_NAME) --network devnet --jito --dry-run

# Run with extreme fast + Jito
.PHONY: run-extreme-jito
run-extreme-jito: build
	@echo "Running $(BINARY_NAME) with Extreme Fast + Jito protection..."
	./$(BUILD_DIR)/$(BINARY_NAME) --extreme-fast --jito

# Test Jito connectivity
.PHONY: test-jito
test-jito:
	@echo "Testing Jito connectivity..."
	@cd scripts && go run test_jito.go

# Test different Jito endpoints
.PHONY: test-jito-regions
test-jito-regions:
	@echo "Testing Jito regional endpoints..."
	@cd scripts && go run test_jito.go global
	@cd scripts && go run test_jito.go frankfurt
	@cd scripts && go run test_jito.go amsterdam
	@cd scripts && go run test_jito.go ny
	@cd scripts && go run test_jito.go tokyo

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

# 🛡️ Comprehensive Jito testing
.PHONY: test-jito-comprehensive
test-jito-comprehensive: test-jito test-jito-regions
	@echo "Running comprehensive Jito tests..."
	@echo "Testing Jito + Bot integration..."
	@timeout 15 ./build/pump-fun-bot --network devnet --jito --dry-run --log-level debug || true
	@echo "Jito comprehensive testing completed"

# 🚀 Quick start commands for different modes
.PHONY: quick-start
quick-start:
	@echo "🚀 Quick Start Commands:"
	@echo ""
	@echo "📋 Basic Commands:"
	@echo "make run-devnet              # Safe devnet testing"
	@echo "make run-jito-devnet         # Test Jito on devnet"
	@echo "make run-debug               # Debug mode"
	@echo ""
	@echo "🛡️ Jito Commands:"
	@echo "make test-jito               # Test Jito connectivity"
	@echo "make run-jito                # Run with Jito protection"
	@echo "make run-extreme-jito        # Extreme fast + Jito"
	@echo ""
	@echo "⚡ Performance Commands:"
	@echo "make run-hold                # Hold-only mode"
	@echo "make run-yolo                # YOLO mode (dangerous!)"
	@echo ""
	@echo "🔧 Development Commands:"
	@echo "make test-all                # Run all tests"
	@echo "make debug                   # Quick debugging"
	@echo "make test-jito-comprehensive # Full Jito testing"

# Help with Jito-specific examples
.PHONY: help-jito
help-jito:
	@echo "🛡️ Jito MEV Protection Commands:"
	@echo ""
	@echo "Basic Jito usage:"
	@echo "  make test-jito                              # Test Jito connection"
	@echo "  make run-jito-devnet                        # Safe testing on devnet"
	@echo "  make run-jito                               # Live trading with MEV protection"
	@echo ""
	@echo "Advanced Jito usage:"
	@echo "  ./build/pump-fun-bot --jito --jito-tip 15000          # Custom tip amount"
	@echo "  ./build/pump-fun-bot --extreme-fast --jito             # Speed + protection"
	@echo "  ./build/pump-fun-bot --jito --match 'doge'             # Filtered + protected"
	@echo ""
	@echo "Regional endpoint testing:"
	@echo "  make test-jito-regions                      # Test all regional endpoints"
	@echo ""
	@echo "Environment variable examples:"
	@echo "  PUMPBOT_JITO_ENABLED=true PUMPBOT_JITO_TIP_AMOUNT=20000 make run"
	@echo "  PUMPBOT_JITO_ENDPOINT=https://frankfurt.mainnet.block-engine.jito.wtf/api/v1/bundles make run-jito"
	@echo ""
	@echo "💡 Tips:"
	@echo "  - Always test with --dry-run first"
	@echo "  - Use regional endpoints for better performance"
	@echo "  - Start with small tip amounts (10000-15000 lamports)"
	@echo "  - Monitor bundle success rates"
	@echo "  - Enable fallback to regular transactions"

# Help
.PHONY: help
help:
	@echo "Available commands:"
	@echo ""
	@echo "🏗️  Build & Run:"
	@echo "  build        - Build the application"
	@echo "  build-all    - Build for multiple platforms"
	@echo "  run          - Build and run the application"
	@echo "  run-devnet   - Run on devnet"
	@echo "  run-debug    - Run with debug logging"
	@echo ""
	@echo "🛡️  Jito MEV Protection:"
	@echo "  run-jito              - Run with Jito MEV protection"
	@echo "  run-jito-devnet       - Test Jito on devnet"
	@echo "  run-extreme-jito      - Extreme fast + Jito protection"
	@echo "  test-jito             - Test Jito connectivity"
	@echo "  test-jito-regions     - Test regional endpoints"
	@echo "  help-jito             - Detailed Jito help"
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
	@echo "  quick-start  - Show quick start commands"
	@echo "  help-jito    - Show Jito-specific help"

# Example usage targets with Jito
.PHONY: examples
examples:
	@echo "🚀 Example Usage Commands:"
	@echo ""
	@echo "🔰 Beginner (Safe Testing):"
	@echo "make run-devnet"
	@echo "# OR: ./build/pump-fun-bot --network devnet --dry-run"
	@echo ""
	@echo "🛡️ MEV Protected Trading:"
	@echo "make run-jito"
	@echo "# OR: ./build/pump-fun-bot --network mainnet --jito --jito-tip 15000"
	@echo ""
	@echo "⚡ Extreme Speed + MEV Protection:"
	@echo "make run-extreme-jito"
	@echo "# OR: ./build/pump-fun-bot --network mainnet --extreme-fast --jito"
	@echo ""
	@echo "🎯 Filtered Trading with Protection:"
	@echo "./build/pump-fun-bot --network mainnet --match 'doge' --jito --hold"
	@echo ""
	@echo "💰 High-Frequency Trading:"
	@echo "./build/pump-fun-bot --network mainnet --yolo --extreme-fast --jito --jito-tip 25000"
	@echo ""
	@echo "🌍 Regional Optimization (Europe):"
	@echo "PUMPBOT_JITO_ENDPOINT=https://frankfurt.mainnet.block-engine.jito.wtf/api/v1/bundles ./build/pump-fun-bot --jito"
	@echo ""
	@echo "🔧 Custom Configuration:"
	@echo "PUMPBOT_TRADING_BUY_AMOUNT_SOL=0.05 PUMPBOT_JITO_TIP_AMOUNT=20000 make run-jito"
	@echo ""
	@echo "📊 Testing & Development:"
	@echo "make test-jito                                    # Test Jito connectivity"
	@echo "./build/pump-fun-bot --network devnet --jito --dry-run --log-level debug"
	@echo ""
	@echo "⚠️  Always start with devnet and --dry-run for testing!"

# Performance benchmarking for Jito
.PHONY: bench-jito
bench-jito:
	@echo "🏃‍♂️ Benchmarking Jito performance..."
	@echo "Testing different tip amounts and their success rates..."
	@cd scripts && go run test_jito.go
	@echo ""
	@echo "Running bot performance test..."
	@timeout 30 ./build/pump-fun-bot --network devnet --jito --dry-run --log-level info || true
	@echo "Jito performance benchmarking completed"

# Create example environment files
.PHONY: create-env-examples
create-env-examples:
	@echo "Creating example environment files..."
	@mkdir -p configs

	@echo "# Conservative setup for beginners" > configs/.env.conservative
	@echo "PUMPBOT_NETWORK=devnet" >> configs/.env.conservative
	@echo "PUMPBOT_TRADING_BUY_AMOUNT_SOL=0.001" >> configs/.env.conservative
	@echo "PUMPBOT_STRATEGY_HOLD_ONLY=true" >> configs/.env.conservative
	@echo "PUMPBOT_LOGGING_LEVEL=info" >> configs/.env.conservative

	@echo "# MEV Protected trading" > configs/.env.jito
	@echo "PUMPBOT_NETWORK=mainnet" >> configs/.env.jito
	@echo "PUMPBOT_JITO_ENABLED=true" >> configs/.env.jito
	@echo "PUMPBOT_JITO_USE_FOR_TRADING=true" >> configs/.env.jito
	@echo "PUMPBOT_JITO_TIP_AMOUNT=15000" >> configs/.env.jito
	@echo "PUMPBOT_TRADING_BUY_AMOUNT_SOL=0.01" >> configs/.env.jito

	@echo "# Extreme fast + Jito protection" > configs/.env.extreme
	@echo "PUMPBOT_NETWORK=mainnet" >> configs/.env.extreme
	@echo "PUMPBOT_EXTREME_FAST_ENABLED=true" >> configs/.env.extreme
	@echo "PUMPBOT_EXTREME_FAST_USE_JITO=true" >> configs/.env.extreme
	@echo "PUMPBOT_JITO_ENABLED=true" >> configs/.env.extreme
	@echo "PUMPBOT_JITO_USE_FOR_TRADING=true" >> configs/.env.extreme
	@echo "PUMPBOT_JITO_TIP_AMOUNT=20000" >> configs/.env.extreme
	@echo "PUMPBOT_EXTREME_FAST_PRIORITY_FEE=200000" >> configs/.env.extreme

	@echo "# Aggressive trading (high risk)" > configs/.env.aggressive
	@echo "PUMPBOT_NETWORK=mainnet" >> configs/.env.aggressive
	@echo "PUMPBOT_STRATEGY_YOLO_MODE=true" >> configs/.env.aggressive
	@echo "PUMPBOT_TRADING_BUY_AMOUNT_SOL=0.05" >> configs/.env.aggressive
	@echo "PUMPBOT_EXTREME_FAST_ENABLED=true" >> configs/.env.aggressive
	@echo "PUMPBOT_JITO_ENABLED=true" >> configs/.env.aggressive
	@echo "PUMPBOT_JITO_AUTO_TIP_ADJUSTMENT=true" >> configs/.env.aggressive

	@echo "✅ Created example environment files:"
	@echo "  - configs/.env.conservative  (Safe for beginners)"
	@echo "  - configs/.env.jito          (MEV protected)"
	@echo "  - configs/.env.extreme       (Speed + protection)"
	@echo "  - configs/.env.aggressive    (High risk/reward)"
	@echo ""
	@echo "Usage: ./build/pump-fun-bot --env configs/.env.jito"

# Comprehensive setup for new users
.PHONY: setup-complete
setup-complete: setup create-env-examples
	@echo ""
	@echo "🎉 Complete setup finished!"
	@echo ""
	@echo "📋 Next steps:"
	@echo "1. Copy .env.example to .env and set your PUMPBOT_PRIVATE_KEY"
	@echo "2. Test the setup: make test-devnet"
	@echo "3. Test Jito: make test-jito"
	@echo "4. Start trading: make run-jito-devnet"
	@echo ""
	@echo "📚 Learn more:"
	@echo "- make help-jito     # Jito-specific commands"
	@echo "- make examples      # Usage examples"
	@echo "- make quick-start   # Quick start guide"

# Validate current configuration
.PHONY: validate-config
validate-config:
	@echo "🔍 Validating current configuration..."
	@if [ -z "$PUMPBOT_PRIVATE_KEY" ]; then \
		echo "❌ PUMPBOT_PRIVATE_KEY not set"; \
		echo "Set it with: export PUMPBOT_PRIVATE_KEY='your_key_here'"; \
		exit 1; \
	else \
		echo "✅ Private key is set"; \
	fi

	@echo "Network: ${PUMPBOT_NETWORK:-mainnet}"
	@echo "Buy Amount: ${PUMPBOT_TRADING_BUY_AMOUNT_SOL:-0.01} SOL"
	@echo "Jito Enabled: ${PUMPBOT_JITO_ENABLED:-false}"
	@echo "Extreme Fast: ${PUMPBOT_EXTREME_FAST_ENABLED:-false}"
	@echo ""
	@echo "✅ Configuration validation complete"

# Show current costs estimation
.PHONY: estimate-costs
estimate-costs:
	@echo "💰 Trading Cost Estimation"
	@echo "=========================="
	@echo ""
	@echo "Based on current configuration:"
	@echo "Buy Amount: ${PUMPBOT_TRADING_BUY_AMOUNT_SOL:-0.01} SOL"
	@echo ""
	@echo "Cost breakdown per trade:"
	@echo "- Base transaction fee: ~0.000005 SOL"
	@echo "- Priority fee: ~0.000010-0.000050 SOL"
	@if [ "${PUMPBOT_JITO_ENABLED}" = "true" ]; then \
		tip_sol=$(echo "scale=6; ${PUMPBOT_JITO_TIP_AMOUNT:-10000} / 1000000000" | bc -l 2>/dev/null || echo "0.00001"); \
		echo "- Jito tip: ~$tip_sol SOL"; \
	fi
	@if [ "${PUMPBOT_EXTREME_FAST_ENABLED}" = "true" ]; then \
		echo "- Extreme fast fees: ~0.000100-0.000400 SOL"; \
	fi
	@echo ""
	@echo "Estimated total: 0.000015-0.000455 SOL per trade"
	@echo "As percentage of buy amount: 0.15%-4.55%"
	@echo ""
	@echo "💡 Tips to reduce costs:"
	@echo "- Use lower priority fees during low congestion"
	@echo "- Adjust Jito tips based on network conditions"
	@echo "- Use regional Jito endpoints for better performance"
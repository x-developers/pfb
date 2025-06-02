#!/bin/bash

# Pump Fun Bot - Enhanced Run Script with .env support
# This script helps you run the bot with proper diagnostics and environment management

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

# Print functions
print_header() {
    echo -e "${PURPLE}================================${NC}"
    echo -e "${PURPLE} 🚀 Pump Fun Bot Runner v2.1${NC}"
    echo -e "${PURPLE}================================${NC}"
    echo ""
}

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_step() {
    echo -e "${CYAN}$1${NC}"
}

# Load environment variables from .env file (for script-level usage)
load_env_file() {
    local env_file="$1"

    if [ -f "$env_file" ]; then
        print_info "Pre-loading environment from $env_file for script"

        # Export variables from .env file, handling comments and empty lines
        while IFS= read -r line || [ -n "$line" ]; do
            # Skip comments and empty lines
            [[ "$line" =~ ^[[:space:]]*# ]] && continue
            [[ -z "${line// }" ]] && continue

            # Export the variable if it contains =
            if [[ "$line" == *"="* ]]; then
                export "$line"
            fi
        done < "$env_file"

        print_success "Environment pre-loaded for script operations"

        # Debug: Show loaded RPC/WS URLs if they exist
        if [ -n "$PUMPBOT_RPC_URL" ]; then
            print_info "Script loaded RPC URL: $PUMPBOT_RPC_URL"
        fi
        if [ -n "$PUMPBOT_WS_URL" ]; then
            print_info "Script loaded WebSocket URL: $PUMPBOT_WS_URL"
        fi
    else
        print_warning ".env file not found at $env_file"
        print_info "The bot will look for .env files in default locations"
    fi
}

# Show usage
show_usage() {
    echo "Usage: $0 [mode] [options]"
    echo ""
    echo "Modes:"
    echo "  debug       - Run diagnostics and debug"
    echo "  test        - Test run (dry-run on devnet)"
    echo "  devnet      - Live run on devnet"
    echo "  mainnet     - Live run on mainnet (⚠️  REAL MONEY!)"
    echo "  ws-debug    - Debug WebSocket connection only"
    echo "  env-check   - Check environment variables"
    echo ""
    echo "Options:"
    echo "  --env FILE         - Load specific .env file"
    echo "  --match PATTERN    - Filter tokens by name pattern"
    echo "  --creator ADDRESS  - Filter tokens by creator"
    echo "  --hold            - Hold only mode (no selling)"
    echo "  --yolo            - Continuous trading mode"
    echo "  --amount SOL      - Buy amount in SOL"
    echo "  --config FILE     - Use specific config file"
    echo "  --rpc-url URL     - Custom RPC endpoint"
    echo "  --ws-url URL      - Custom WebSocket endpoint"
    echo "  --extreme-fast    - Enable extreme fast mode"
    echo ""
    echo "Environment Files:"
    echo "  .env              - Default environment file"
    echo "  .env.development  - Development environment"
    echo "  .env.production   - Production environment"
    echo ""
    echo "Examples:"
    echo "  $0 debug                              # Run diagnostics"
    echo "  $0 test --env .env.development        # Test with dev env"
    echo "  $0 devnet --match doge --amount 0.01  # Live devnet with settings"
    echo "  $0 mainnet --env .env.production      # Use production .env file"
    echo "  $0 devnet --env configs/.env.devnet   # Custom .env location"
    echo "  $0 env-check                         # Check environment setup"
    echo ""
}

# Check environment variables
check_environment() {
    print_step "🔍 Checking environment configuration..."

    local errors=0

    # Check required variables
    if [ -z "$PUMPBOT_PRIVATE_KEY" ]; then
        print_error "PUMPBOT_PRIVATE_KEY is not set"
        errors=$((errors + 1))
    else
        print_success "PUMPBOT_PRIVATE_KEY is set"
    fi

    # Check network setting
    local network="${PUMPBOT_NETWORK:-mainnet}"
    if [ "$network" != "mainnet" ] && [ "$network" != "devnet" ]; then
        print_error "PUMPBOT_NETWORK must be 'mainnet' or 'devnet', got: $network"
        errors=$((errors + 1))
    else
        print_success "PUMPBOT_NETWORK is set to: $network"
    fi

    # Check trading amount
    local amount="${PUMPBOT_TRADING_BUY_AMOUNT_SOL:-0.01}"
    if ! [[ "$amount" =~ ^[0-9]+\.?[0-9]*$ ]]; then
        print_error "PUMPBOT_TRADING_BUY_AMOUNT_SOL must be a valid number, got: $amount"
        errors=$((errors + 1))
    else
        print_success "PUMPBOT_TRADING_BUY_AMOUNT_SOL is set to: $amount SOL"
    fi

    # Show current configuration
    echo ""
    print_info "Current configuration:"
    echo "  Network: ${PUMPBOT_NETWORK:-mainnet}"
    echo "  RPC URL: ${PUMPBOT_RPC_URL:-[default for network]}"
    echo "  WebSocket URL: ${PUMPBOT_WS_URL:-[default for network]}"
    echo "  Buy Amount: ${PUMPBOT_TRADING_BUY_AMOUNT_SOL:-0.01} SOL"
    echo "  Strategy: ${PUMPBOT_STRATEGY_TYPE:-sniper}"
    echo "  Log Level: ${PUMPBOT_LOGGING_LEVEL:-info}"
    echo "  YOLO Mode: ${PUMPBOT_STRATEGY_YOLO_MODE:-false}"
    echo "  Hold Only: ${PUMPBOT_STRATEGY_HOLD_ONLY:-false}"
    echo "  Extreme Fast: ${PUMPBOT_EXTREME_FAST_ENABLED:-false}"
    echo ""

    if [ $errors -gt 0 ]; then
        print_error "$errors environment error(s) found"
        print_info "Create a .env file with required variables or set them manually"
        print_info "See .env.example for reference"
        return 1
    fi

    return 0
}

# Show detailed environment check
show_env_check() {
    print_step "🔧 Environment Variables Check"
    echo ""

    # Required variables
    echo "📋 Required Variables:"
    check_var "PUMPBOT_PRIVATE_KEY" "${PUMPBOT_PRIVATE_KEY:-}" "hidden" true

    # Network variables
    echo ""
    echo "🌐 Network Variables:"
    check_var "PUMPBOT_NETWORK" "${PUMPBOT_NETWORK:-mainnet}" "visible" false
    check_var "PUMPBOT_RPC_URL" "${PUMPBOT_RPC_URL:-}" "visible" false
    check_var "PUMPBOT_WS_URL" "${PUMPBOT_WS_URL:-}" "visible" false
    check_var "PUMPBOT_RPC_API_KEY" "${PUMPBOT_RPC_API_KEY:-}" "hidden" false

    # Trading variables
    echo ""
    echo "💰 Trading Variables:"
    check_var "PUMPBOT_TRADING_BUY_AMOUNT_SOL" "${PUMPBOT_TRADING_BUY_AMOUNT_SOL:-0.01}" "visible" false
    check_var "PUMPBOT_TRADING_SLIPPAGE_BP" "${PUMPBOT_TRADING_SLIPPAGE_BP:-500}" "visible" false
    check_var "PUMPBOT_TRADING_AUTO_SELL" "${PUMPBOT_TRADING_AUTO_SELL:-false}" "visible" false
    check_var "PUMPBOT_TRADING_PRIORITY_FEE" "${PUMPBOT_TRADING_PRIORITY_FEE:-0}" "visible" false

    # Strategy variables
    echo ""
    echo "🎯 Strategy Variables:"
    check_var "PUMPBOT_STRATEGY_TYPE" "${PUMPBOT_STRATEGY_TYPE:-sniper}" "visible" false
    check_var "PUMPBOT_STRATEGY_YOLO_MODE" "${PUMPBOT_STRATEGY_YOLO_MODE:-false}" "visible" false
    check_var "PUMPBOT_STRATEGY_HOLD_ONLY" "${PUMPBOT_STRATEGY_HOLD_ONLY:-false}" "visible" false

    # Extreme Fast Mode variables
    echo ""
    echo "⚡ Extreme Fast Mode Variables:"
    check_var "PUMPBOT_EXTREME_FAST_ENABLED" "${PUMPBOT_EXTREME_FAST_ENABLED:-false}" "visible" false
    check_var "PUMPBOT_EXTREME_FAST_PRIORITY_FEE" "${PUMPBOT_EXTREME_FAST_PRIORITY_FEE:-100000}" "visible" false
    check_var "PUMPBOT_EXTREME_FAST_COMPUTE_LIMIT" "${PUMPBOT_EXTREME_FAST_COMPUTE_LIMIT:-400000}" "visible" false

    # Logging variables
    echo ""
    echo "📝 Logging Variables:"
    check_var "PUMPBOT_LOGGING_LEVEL" "${PUMPBOT_LOGGING_LEVEL:-info}" "visible" false
    check_var "PUMPBOT_LOGGING_FORMAT" "${PUMPBOT_LOGGING_FORMAT:-text}" "visible" false
}

# Check individual variable
check_var() {
    local name="$1"
    local value="$2"
    local visibility="$3"
    local required="$4"

    if [ -n "$value" ]; then
        if [ "$visibility" = "hidden" ]; then
            print_success "$name: [SET]"
        else
            print_success "$name: $value"
        fi
    else
        if [ "$required" = "true" ]; then
            print_error "$name: [NOT SET] (REQUIRED)"
        else
            print_info "$name: [NOT SET] (optional)"
        fi
    fi
}

# Check prerequisites
check_prerequisites() {
    print_step "🔍 Checking prerequisites..."

    # Check if bot is built
    if [ ! -f "./build/pump-fun-bot" ]; then
        print_info "Building bot..."
        make build > /dev/null 2>&1
        if [ $? -ne 0 ]; then
            print_error "Failed to build bot"
            exit 1
        fi
        print_success "Bot built successfully"
    else
        print_success "Bot binary found"
    fi

    # Check environment
    if ! check_environment; then
        exit 1
    fi

    echo ""
}

# Run diagnostics
run_diagnostics() {
    print_step "🩺 Running diagnostics..."
    make debug
    echo ""
}

# Run WebSocket debug
run_ws_debug() {
    print_step "🔌 Running WebSocket diagnostics..."
    make debug-ws
}

# Parse command line arguments
parse_args() {
    MODE=""
    BOT_ARGS=""
    ENV_FILE=".env"

    # Parse mode
    if [ $# -eq 0 ]; then
        show_usage
        exit 1
    fi

    MODE=$1
    shift

    # Parse additional arguments
    while [ $# -gt 0 ]; do
        case $1 in
            --env)
                ENV_FILE="$2"
                BOT_ARGS="$BOT_ARGS --env $2"
                shift 2
                ;;
            --match)
                BOT_ARGS="$BOT_ARGS --match $2"
                shift 2
                ;;
            --creator)
                BOT_ARGS="$BOT_ARGS --creator $2"
                shift 2
                ;;
            --hold)
                BOT_ARGS="$BOT_ARGS --hold"
                export PUMPBOT_STRATEGY_HOLD_ONLY=true
                shift
                ;;
            --yolo)
                BOT_ARGS="$BOT_ARGS --yolo"
                export PUMPBOT_STRATEGY_YOLO_MODE=true
                shift
                ;;
            --extreme-fast)
                BOT_ARGS="$BOT_ARGS --extreme-fast"
                export PUMPBOT_EXTREME_FAST_ENABLED=true
                shift
                ;;
            --amount)
                export PUMPBOT_TRADING_BUY_AMOUNT_SOL="$2"
                shift 2
                ;;
            --rpc-url)
                export PUMPBOT_RPC_URL="$2"
                shift 2
                ;;
            --ws-url)
                export PUMPBOT_WS_URL="$2"
                shift 2
                ;;
            --config)
                BOT_ARGS="$BOT_ARGS --config $2"
                shift 2
                ;;
            *)
                print_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done
}

# Test network connectivity
test_network_connectivity() {
    local network="$1"
    local rpc_url="${PUMPBOT_RPC_URL}"
    local ws_url="${PUMPBOT_WS_URL}"

    # Use default URLs if not specified
    if [ -z "$rpc_url" ]; then
        if [ "$network" = "devnet" ]; then
            rpc_url="https://api.devnet.solana.com"
        else
            rpc_url="https://api.mainnet-beta.solana.com"
        fi
    fi

    if [ -z "$ws_url" ]; then
        if [ "$network" = "devnet" ]; then
            ws_url="wss://api.devnet.solana.com"
        else
            ws_url="wss://api.mainnet-beta.solana.com"
        fi
    fi

    print_info "Testing network connectivity..."
    print_info "RPC URL: $rpc_url"
    print_info "WebSocket URL: $ws_url"

    # Test RPC connectivity
    if command -v curl > /dev/null 2>&1; then
        if curl -s -X POST "$rpc_url" \
          -H "Content-Type: application/json" \
          -d '{"jsonrpc":"2.0","id":1,"method":"getHealth"}' > /dev/null; then
            print_success "RPC endpoint is reachable"
        else
            print_warning "RPC endpoint may not be reachable"
        fi
    else
        print_info "curl not available, skipping RPC connectivity test"
    fi

    # Test WebSocket port (extract hostname and test port 443)
    local ws_host=$(echo "$ws_url" | sed 's|wss://||' | sed 's|/.*||')
    if command -v timeout > /dev/null 2>&1; then
        if timeout 3 bash -c "</dev/tcp/$ws_host/443" 2>/dev/null; then
            print_success "WebSocket port is reachable"
        else
            print_warning "WebSocket port may not be reachable"
        fi
    else
        print_info "timeout not available, skipping WebSocket connectivity test"
    fi
}

# Run the bot
run_bot() {
    local network=$1
    local extra_args=$2

    print_step "🚀 Starting Pump Fun Bot..."
    print_info "Network: $network"
    print_info "Arguments: $extra_args"
    print_info "Bot Args: $BOT_ARGS"

    # Test network connectivity
    test_network_connectivity "$network"

    # Show current endpoints being used
    echo ""
    print_info "Endpoints being used:"
    if [ -n "$PUMPBOT_RPC_URL" ]; then
        print_info "RPC URL: $PUMPBOT_RPC_URL (custom)"
    else
        print_info "RPC URL: [default for $network]"
    fi

    if [ -n "$PUMPBOT_WS_URL" ]; then
        print_info "WebSocket URL: $PUMPBOT_WS_URL (custom)"
    else
        print_info "WebSocket URL: [default for $network]"
    fi

    if [ "$network" = "mainnet" ]; then
        print_warning "⚠️  MAINNET MODE - REAL MONEY AT RISK!"
        print_warning "Current settings:"
        print_warning "  Buy Amount: ${PUMPBOT_TRADING_BUY_AMOUNT_SOL:-0.01} SOL"
        print_warning "  YOLO Mode: ${PUMPBOT_STRATEGY_YOLO_MODE:-false}"
        print_warning "  Hold Only: ${PUMPBOT_STRATEGY_HOLD_ONLY:-false}"
        print_warning "  Extreme Fast: ${PUMPBOT_EXTREME_FAST_ENABLED:-false}"
        print_warning ""
        if [ "$extra_args" != "--dry-run" ]; then
            print_warning "⚠️  THIS WILL USE REAL SOL! Press Ctrl+C within 10 seconds to cancel..."
            sleep 10
        fi
    fi

    echo ""
    print_success "🎯 Bot starting - watch for token discoveries!"
    echo ""

    # Run the bot
    ./build/pump-fun-bot --network $network --log-level ${PUMPBOT_LOGGING_LEVEL:-info} $extra_args $BOT_ARGS
}

# Create .env file from template
create_env_file() {
    if [ ! -f ".env" ] && [ -f ".env.example" ]; then
        print_info "Creating .env file from .env.example"
        cp .env.example .env
        print_warning "Please edit .env file with your actual values"
        print_info "Especially set PUMPBOT_PRIVATE_KEY"
        return 1
    fi
    return 0
}

# Main execution
main() {
    print_header

    # Parse arguments
    parse_args "$@"

    # Load environment file
    load_env_file "$ENV_FILE"

    # Create .env if needed
    if [ "$MODE" != "env-check" ]; then
        create_env_file
    fi

    # Execute based on mode
    case $MODE in
        debug)
            check_prerequisites
            run_diagnostics
            ;;
        test)
            check_prerequisites
            print_step "🧪 Running test mode (dry-run on devnet)..."
            export PUMPBOT_NETWORK=devnet
            run_bot "devnet" "--dry-run"
            ;;
        devnet)
            check_prerequisites
            print_step "🌐 Running on devnet (live trading with test SOL)..."
            export PUMPBOT_NETWORK=devnet
            run_bot "devnet" ""
            ;;
        mainnet)
            check_prerequisites
            print_step "💰 Running on mainnet (REAL MONEY!)..."
            export PUMPBOT_NETWORK=mainnet
            run_bot "mainnet" ""
            ;;
        ws-debug)
            run_ws_debug
            ;;
        env-check)
            show_env_check
            ;;
        help|--help|-h)
            show_usage
            ;;
        *)
            print_error "Unknown mode: $MODE"
            show_usage
            exit 1
            ;;
    esac
}

# Trap Ctrl+C for graceful shutdown
trap 'echo -e "\n🛑 Gracefully shutting down..."; exit 0' INT

# Run main function
main "$@"
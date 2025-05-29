#!/bin/bash

# Pump Fun Bot - Main Run Script
# This script helps you run the bot with proper diagnostics

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
    echo -e "${PURPLE} 🚀 Pump Fun Bot Runner${NC}"
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
    echo ""
    echo "Options:"
    echo "  --match PATTERN    - Filter tokens by name pattern"
    echo "  --creator ADDRESS  - Filter tokens by creator"
    echo "  --hold            - Hold only mode (no selling)"
    echo "  --yolo            - Continuous trading mode"
    echo "  --amount SOL      - Buy amount in SOL"
    echo ""
    echo "Examples:"
    echo "  $0 debug                           # Run diagnostics"
    echo "  $0 test                           # Safe test run"
    echo "  $0 devnet --match doge            # Live devnet with filter"
    echo "  $0 devnet --hold --amount 0.001   # Conservative devnet run"
    echo ""
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

    # Check private key
    if [ -z "$PUMPBOT_PRIVATE_KEY" ]; then
        print_error "PUMPBOT_PRIVATE_KEY environment variable is required"
        echo ""
        echo "Set it with:"
        echo "  export PUMPBOT_PRIVATE_KEY='your_solana_private_key_here'"
        echo ""
        echo "For devnet testing, you can generate a test key with:"
        echo "  solana-keygen new --outfile test-keypair.json"
        echo "  export PUMPBOT_PRIVATE_KEY=\$(cat test-keypair.json | jq -r '.[0:32] | @base64')"
        exit 1
    fi
    print_success "Private key found"

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
                shift
                ;;
            --yolo)
                BOT_ARGS="$BOT_ARGS --yolo"
                shift
                ;;
            --amount)
                export PUMPBOT_TRADING_BUY_AMOUNT_SOL="$2"
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

# Run the bot
run_bot() {
    local network=$1
    local extra_args=$2

    print_step "🚀 Starting Pump Fun Bot..."
    print_info "Network: $network"
    print_info "Arguments: $extra_args"

    if [ "$network" = "mainnet" ]; then
        print_warning "⚠️  MAINNET MODE - REAL MONEY AT RISK!"
        print_warning "Press Ctrl+C within 5 seconds to cancel..."
        sleep 5
    fi

    echo ""
    print_success "🎯 Bot starting - watch for token discoveries!"
    echo ""

    # Run the bot
    ./build/pump-fun-bot --network $network --log-level info $extra_args $BOT_ARGS
}

# Main execution
main() {
    print_header

    # Parse arguments
    parse_args "$@"

    # Check prerequisites
    check_prerequisites

    # Execute based on mode
    case $MODE in
        debug)
            run_diagnostics
            ;;
        test)
            print_step "🧪 Running test mode (dry-run on devnet)..."
            run_bot "devnet" "--dry-run"
            ;;
        devnet)
            print_step "🌐 Running on devnet (live trading with test SOL)..."
            run_bot "devnet" ""
            ;;
        mainnet)
            print_step "💰 Running on mainnet (REAL MONEY!)..."
            run_bot "mainnet" ""
            ;;
        ws-debug)
            run_ws_debug
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
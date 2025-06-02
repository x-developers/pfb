#!/bin/bash

# Quick Debug Script for Pump Fun Bot
echo "🔍 Pump Fun Bot - Quick Debug"
echo "=============================="

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if binary exists
if [ ! -f "./build/pump-fun-bot" ]; then
    print_info "Building bot..."
    make build > /dev/null 2>&1
    if [ $? -ne 0 ]; then
        print_error "Failed to build bot"
        exit 1
    fi
fi

# Check private key
if [ -z "$PUMPBOT_PRIVATE_KEY" ]; then
    print_error "PUMPBOT_PRIVATE_KEY environment variable not set"
    echo "Please set it with: export PUMPBOT_PRIVATE_KEY='your_key_here'"
    exit 1
fi
print_success "Private key found"

# Test network connectivity
print_info "Testing network connectivity..."

# Test devnet RPC
curl -s -X POST https://api.devnet.solana.com \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"getHealth"}' > /dev/null

if [ $? -eq 0 ]; then
    print_success "Devnet RPC reachable"
else
    print_error "Devnet RPC not reachable"
fi

# Test devnet WebSocket (check if port 443 is open)
timeout 3 bash -c "</dev/tcp/api.devnet.solana.com/443" 2>/dev/null
if [ $? -eq 0 ]; then
    print_success "Devnet WebSocket port reachable"
else
    print_warning "Devnet WebSocket port may not be reachable"
fi

# Test bot startup
print_info "Testing bot startup (5 seconds)..."
timeout 5 ./build/pump-fun-bot --network devnet --log-level debug --dry-run > debug_output.log 2>&1 &
BOT_PID=$!

sleep 5
kill $BOT_PID 2>/dev/null || true
wait $BOT_PID 2>/dev/null || true

# Analyze log output
if [ -f "debug_output.log" ]; then
    print_info "Analyzing debug output..."

    # Check for successful startup
    if grep -q "Bot starting up" debug_output.log; then
        print_success "Bot startup logged"
    else
        print_error "Bot startup not found in logs"
    fi

    # Check for WebSocket connection
    if grep -q "WebSocket connected" debug_output.log; then
        print_success "WebSocket connection established"
    else
        print_warning "WebSocket connection not found in logs"
    fi

    # Check for subscription
    if grep -q "Subscribed to pump.fun" debug_output.log; then
        print_success "Subscription to pump.fun logs successful"
    else
        print_warning "Subscription to pump.fun logs not found"
    fi

    # Check for errors
    if grep -qi "error\|failed\|panic" debug_output.log; then
        print_error "Errors found in logs:"
        grep -i "error\|failed\|panic" debug_output.log | head -5
    else
        print_success "No obvious errors in logs"
    fi

    # Check for notifications
    if grep -q "notification\|logs notification" debug_output.log; then
        print_success "WebSocket notifications received"
    else
        print_warning "No WebSocket notifications found (may be normal if no activity)"
    fi

    echo ""
    print_info "Last 10 lines of debug output:"
    tail -10 debug_output.log

    echo ""
    print_info "Full debug log saved to: debug_output.log"
else
    print_error "No debug output found"
fi

echo ""
print_info "Debug completed. If issues persist:"
echo "1. Check your private key is valid for devnet"
echo "2. Try: make debug-ws  # for WebSocket-specific debugging"
echo "3. Try: ./build/pump-fun-bot --network devnet --log-level debug"
echo "4. Check your internet connection and firewall settings"
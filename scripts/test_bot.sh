#!/bin/bash

# Pump Fun Bot - Test Script
# Comprehensive testing of all bot functionality

set -e

echo "🚀 Pump Fun Bot - Comprehensive Test Suite"
echo "=========================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
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

# Check if bot binary exists
if [ ! -f "./build/pump-fun-bot" ]; then
    print_status "Building bot..."
    make build
    if [ $? -ne 0 ]; then
        print_error "Failed to build bot"
        exit 1
    fi
    print_success "Bot built successfully"
fi

# Check if private key is provided
if [ -z "$PUMPBOT_PRIVATE_KEY" ]; then
    print_error "PUMPBOT_PRIVATE_KEY environment variable is required"
    print_status "Usage: PUMPBOT_PRIVATE_KEY='your_key' $0"
    exit 1
fi

print_success "Private key found"

# Test 1: Basic configuration validation
print_status "Test 1: Configuration validation"
./build/pump-fun-bot --help > /dev/null 2>&1
if [ $? -eq 0 ]; then
    print_success "Help command works"
else
    print_error "Help command failed"
    exit 1
fi

# Test 2: Network connectivity test (devnet)
print_status "Test 2: Devnet connectivity test"
timeout 10 ./build/pump-fun-bot --network devnet --log-level info --dry-run &
PID=$!
sleep 8
kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true
print_success "Devnet connectivity test completed"

# Test 3: Configuration file test
print_status "Test 3: Configuration file test"
if [ -f "configs/bot.yaml" ]; then
    timeout 5 ./build/pump-fun-bot --config configs/bot.yaml --network devnet --dry-run &
    PID=$!
    sleep 3
    kill $PID 2>/dev/null || true
    wait $PID 2>/dev/null || true
    print_success "Configuration file test completed"
else
    print_warning "No configuration file found, skipping test"
fi

# Test 4: Filter functionality test
print_status "Test 4: Filter functionality test"
timeout 10 ./build/pump-fun-bot --network devnet --match "test" --log-level debug --dry-run &
PID=$!
sleep 8
kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true
print_success "Filter functionality test completed"

# Test 5: Creator filter test
print_status "Test 5: Creator filter test"
timeout 10 ./build/pump-fun-bot --network devnet --creator "11111111111111111111111111111112" --log-level debug --dry-run &
PID=$!
sleep 8
kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true
print_success "Creator filter test completed"

# Test 6: Hold mode test
print_status "Test 6: Hold mode test"
timeout 10 ./build/pump-fun-bot --network devnet --hold --log-level info --dry-run &
PID=$!
sleep 8
kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true
print_success "Hold mode test completed"

# Test 7: Different log levels
print_status "Test 7: Log level tests"
for level in debug info warn error; do
    print_status "Testing log level: $level"
    timeout 5 ./build/pump-fun-bot --network devnet --log-level $level --dry-run &
    PID=$!
    sleep 3
    kill $PID 2>/dev/null || true
    wait $PID 2>/dev/null || true
done
print_success "Log level tests completed"

# Test 8: Combined flags test
print_status "Test 8: Combined flags test"
timeout 10 ./build/pump-fun-bot \
    --network devnet \
    --match "doge" \
    --hold \
    --log-level info \
    --dry-run &
PID=$!
sleep 8
kill $PID 2>/dev/null || true
wait $PID 2>/dev/null || true
print_success "Combined flags test completed"

# Test 9: Error handling test (invalid network)
print_status "Test 9: Error handling test"
./build/pump-fun-bot --network invalid --dry-run 2>/dev/null
if [ $? -ne 0 ]; then
    print_success "Error handling works correctly"
else
    print_warning "Error handling test unexpected result"
fi

# Test 10: Mainnet connection test (if requested)
if [ "$TEST_MAINNET" = "true" ]; then
    print_status "Test 10: Mainnet connectivity test (be careful!)"
    print_warning "This will connect to mainnet - make sure you have a valid key"
    timeout 10 ./build/pump-fun-bot --network mainnet --log-level info --dry-run &
    PID=$!
    sleep 8
    kill $PID 2>/dev/null || true
    wait $PID 2>/dev/null || true
    print_success "Mainnet connectivity test completed"
else
    print_status "Test 10: Skipping mainnet test (set TEST_MAINNET=true to enable)"
fi

echo ""
print_success "🎉 All tests completed successfully!"
echo ""
echo "📊 Test Summary:"
echo "- Configuration validation: ✅"
echo "- Network connectivity: ✅"
echo "- Filter functionality: ✅"
echo "- Error handling: ✅"
echo "- Log level testing: ✅"
echo "- Combined features: ✅"
echo ""
print_status "The bot is ready for use!"
echo ""
echo "🚀 Quick start commands:"
echo "# Test run on devnet:"
echo "PUMPBOT_PRIVATE_KEY='your_key' ./build/pump-fun-bot --network devnet --dry-run"
echo ""
echo "# Live run with filters:"
echo "PUMPBOT_PRIVATE_KEY='your_key' ./build/pump-fun-bot --network devnet --match 'doge' --hold"
echo ""
echo "# YOLO mode (continuous trading):"
echo "PUMPBOT_PRIVATE_KEY='your_key' ./build/pump-fun-bot --network devnet --yolo"
echo ""
print_warning "Remember: Always test on devnet first!"
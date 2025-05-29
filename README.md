# Pump Fun Bot Go

A high-performance pump.fun trading bot written in Go, ported from the original Python implementation.

## Features

- 🚀 **High Performance**: Built with Go for minimal latency and maximum throughput
- 🔄 **Real-time Monitoring**: WebSocket connections for instant new token detection
- 🛡️ **Multiple Strategies**: Sniper and holder trading strategies
- 🔍 **Advanced Filtering**: Filter by creator, token name patterns, and more
- 📊 **Comprehensive Logging**: Structured logging with trade tracking
- ⚙️ **Flexible Configuration**: YAML config files and environment variables
- 🐳 **Docker Support**: Easy deployment with Docker containers

## Quick Start

### Prerequisites

- Go 1.21 or higher
- A Solana wallet with some SOL for trading
- Access to Solana RPC endpoint (mainnet or devnet)

### Installation

```bash
# Clone the repository
git clone https://github.com/your-username/pump-fun-bot-go
cd pump-fun-bot-go

# Install dependencies
make deps

# Setup development environment
make setup
```

### Configuration

1. **Copy the example configuration:**
   ```bash
   cp configs/bot.yaml.example configs/bot.yaml
   ```

2. **Set your private key** (REQUIRED):
   ```bash
   export PUMPBOT_PRIVATE_KEY="your_private_key_here"
   ```

3. **Edit configuration file** `configs/bot.yaml` as needed

### Usage

```bash
# Basic usage - one trade then exit
make run

# YOLO mode - continuous trading
make run-yolo

# Hold only mode - buy but never sell
make run-hold

# Filter by token name
./build/pump-fun-bot --match "doge"

# Filter by creator address
./build/pump-fun-bot --creator "7YmjpX4sPPw9pq6P2hrq9LehAi6QjELPWZYKXRrLaLCB"

# Combined filters
./build/pump-fun-bot --match "pepe" --creator "7Ymj..." --hold --yolo
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PUMPBOT_PRIVATE_KEY` | Your wallet private key (REQUIRED) | - |
| `PUMPBOT_NETWORK` | Network to use (mainnet/devnet) | mainnet |
| `PUMPBOT_TRADING_BUY_AMOUNT_SOL` | SOL amount per trade | 0.01 |
| `PUMPBOT_TRADING_SLIPPAGE_BP` | Slippage tolerance (basis points) | 500 |
| `PUMPBOT_STRATEGY_YOLO_MODE` | Enable continuous trading | false |
| `PUMPBOT_STRATEGY_HOLD_ONLY` | Only buy, never sell | false |

### Configuration File

The bot uses a YAML configuration file located at `configs/bot.yaml`. See the example configuration for all available options.

### Command Line Flags

| Flag | Description |
|------|-------------|
| `--config` | Path to config file |
| `--yolo` | Enable YOLO mode (continuous trading) |
| `--hold` | Hold only mode (no selling) |
| `--match` | Filter tokens by name pattern |
| `--creator` | Filter tokens by creator address |
| `--network` | Network to use (mainnet/devnet) |
| `--log-level` | Log level (debug/info/warn/error) |
| `--dry-run` | Dry run mode (no actual trades) |

## Architecture

### Project Structure

```
pump-fun-bot-go/
├── cmd/bot/              # Application entry point
├── internal/             # Internal packages
│   ├── config/          # Configuration management
│   ├── solana/          # Solana RPC client
│   ├── pumpfun/         # Pump.fun specific logic
│   ├── wallet/          # Wallet management
│   ├── strategy/        # Trading strategies
│   └── logger/          # Logging system
├── pkg/                 # Public packages
│   ├── anchor/          # Anchor framework utilities
│   └── utils/           # General utilities
├── configs/             # Configuration files
├── trades/              # Trade logs
└── logs/                # Application logs
```

### Current Implementation Status

✅ **Phase 1 - Basic Infrastructure** (COMPLETE)
- Go module setup and dependencies
- Configuration system with YAML and environment variables
- Structured logging system with trade tracking
- Basic Solana RPC client
- Application entry point with CLI flags

🚧 **Phase 2 - Solana Integration** (IN PROGRESS)
- WebSocket client for real-time events
- Block subscription and transaction parsing
- Connection to pump.fun program

⏳ **Phase 3 - Anchor Integration** (PLANNED)
- Anchor instruction decoding
- pump.fun IDL integration
- Transaction data extraction

⏳ **Phase 4 - Wallet Management** (PLANNED)
- Private key loading and management
- Associated Token Account creation
- Transaction signing

⏳ **Phase 5 - Trading Logic** (PLANNED)
- Token discovery and filtering
- Buy/sell transaction creation
- Price calculation via bonding curve

⏳ **Phase 6 - Strategies** (PLANNED)
- Sniper strategy implementation
- Holder strategy implementation
- Advanced filtering options

## Trading Strategies

### Sniper Strategy
- **Purpose**: Quick buy/sell for short-term profits
- **Behavior**: Monitors new tokens, buys immediately, sells after delay or profit target
- **Best for**: Catching pump and dumps, quick scalping

### Holder Strategy
- **Purpose**: Long-term token accumulation
- **Behavior**: Buys tokens and holds them, no automatic selling
- **Best for**: Believing in token potential, avoiding day trading

## Development

### Building

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Build and run
make run
```

### Testing

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Run benchmarks
make bench
```

### Code Quality

```bash
# Format code
make fmt

# Lint code
make lint

# Tidy dependencies
make tidy
```

### Development Server

```bash
# Hot reload development server
make dev
```

## Docker

```bash
# Build Docker image
make docker-build

# Run with Docker
make docker-run
```

## Logging

The bot provides comprehensive logging:

- **Application logs**: Structured JSON logs in `logs/bot.log`
- **Trade logs**: Daily trade files in `trades/` directory
- **Performance metrics**: Latency and throughput tracking
- **Position tracking**: Real-time P&L calculation

## Safety Features

- **Dry run mode**: Test strategies without real trades
- **Slippage protection**: Configurable slippage tolerance
- **Rate limiting**: Prevent excessive API calls
- **Error handling**: Comprehensive error handling and retry logic
- **Graceful shutdown**: Clean shutdown on signals

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## Roadmap

- [ ] Complete Phase 2: WebSocket integration
- [ ] Complete Phase 3: Anchor integration
- [ ] Complete Phase 4: Wallet management
- [ ] Complete Phase 5: Trading logic
- [ ] Complete Phase 6: Strategy implementation
- [ ] Add more advanced filtering options
- [ ] Implement portfolio management
- [ ] Add web UI for monitoring
- [ ] Add Telegram/Discord notifications
- [ ] Implement advanced strategies (DCA, etc.)

## Disclaimer

This software is for educational purposes only. Trading cryptocurrencies involves substantial risk and may not be suitable for everyone. You should carefully consider whether trading is suitable for you in light of your circumstances, knowledge, and financial resources. You may lose all or more of your initial investment. Past performance is not indicative of future results.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Original Python implementation: [pump-fun-bot](https://github.com/chainstacklabs/pump-fun-bot)
- Solana Labs for the Solana blockchain
- pump.fun team for the platform
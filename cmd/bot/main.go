package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/blocto/solana-go-sdk/common"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/pumpfun"
	"pump-fun-bot-go/internal/solana"
	"pump-fun-bot-go/internal/wallet"
)

const (
	Version = "1.1.0"
)

// CLI flags
var (
	configPath    = flag.String("config", "", "Path to config file")
	yoloMode      = flag.Bool("yolo", false, "Enable YOLO mode (continuous trading)")
	holdOnly      = flag.Bool("hold", false, "Hold only mode (no selling)")
	matchPattern  = flag.String("match", "", "Filter tokens by name pattern")
	creatorFilter = flag.String("creator", "", "Filter tokens by creator address")
	network       = flag.String("network", "", "Network to use (mainnet/devnet)")
	logLevel      = flag.String("log-level", "", "Log level (debug/info/warn/error)")
	dryRun        = flag.Bool("dry-run", false, "Dry run mode (no actual trades)")
	logsOnly      = flag.Bool("logs-only", false, "Only listen to logs without trading")
)

// Enhanced App with logs-based token discovery
type App struct {
	config       *config.Config
	logger       *logger.Logger
	tradeLogger  *logger.TradeLogger
	solanaClient *solana.Client
	wsClient     *solana.WSClient
	wallet       *wallet.Wallet
	logsListener *pumpfun.Listener
	trader       *pumpfun.Trader
	priceCalc    *pumpfun.PriceCalculator
	ctx          context.Context
	cancel       context.CancelFunc

	// Statistics
	tokensDiscovered int
	tradesAttempted  int
	tradesSuccessful int
}

func main() {
	flag.Parse()

	// Parse CLI arguments
	if err := validateFlags(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	// Load configuration
	cfg, err := loadConfiguration()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Override config with CLI flags
	overrideConfigWithFlags(cfg)

	// Initialize logger
	log, err := initializeLogger(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Create application
	app, err := NewApp(cfg, log)
	if err != nil {
		log.WithError(err).Fatal("Failed to create application")
	}

	// Start application
	if err := app.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start application")
	}
}

// NewApp creates a new enhanced application instance
func NewApp(cfg *config.Config, log *logger.Logger) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize trade logger
	tradeLogger, err := logger.NewTradeLogger(cfg.Logging.TradeLogDir, log)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create trade logger: %w", err)
	}

	// Initialize Solana RPC client
	solanaClient := solana.NewClient(solana.ClientConfig{
		Endpoint: cfg.RPCUrl,
		APIKey:   cfg.RPCAPIKey,
		Timeout:  30 * time.Second,
	}, log.Logger)

	// Initialize WebSocket client
	wsClient := solana.NewWSClient(cfg.WSUrl, log.Logger)

	// Initialize wallet (if not in logs-only mode)
	var walletInstance *wallet.Wallet
	if !*logsOnly && cfg.PrivateKey != "" {
		walletInstance, err = wallet.NewWallet(wallet.WalletConfig{
			PrivateKey: cfg.PrivateKey,
			Network:    cfg.Network,
		}, solanaClient, log.Logger, cfg)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create wallet: %w", err)
		}
	}

	// Initialize price calculator
	priceCalc := pumpfun.NewPriceCalculator(solanaClient)

	// Initialize trader (if not in logs-only mode)
	var trader *pumpfun.Trader
	if !*logsOnly && walletInstance != nil {
		trader = pumpfun.NewTrader(walletInstance, solanaClient, priceCalc, log, tradeLogger, cfg)
	}

	// Initialize logs listener (using the new implementation)
	logsListener := pumpfun.NewListener(wsClient, solanaClient, log, cfg)

	return &App{
		config:       cfg,
		logger:       log,
		tradeLogger:  tradeLogger,
		solanaClient: solanaClient,
		wsClient:     wsClient,
		wallet:       walletInstance,
		logsListener: logsListener,
		trader:       trader,
		priceCalc:    priceCalc,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start starts the enhanced application
func (a *App) Start() error {
	// Log startup information
	a.logger.LogStartup(Version, a.config.Network, a.config.RPCUrl, a.config)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Test connections
	if err := a.testConnections(); err != nil {
		return fmt.Errorf("failed to test connections: %w", err)
	}

	// Start logs listener
	if err := a.logsListener.Start(); err != nil {
		return fmt.Errorf("failed to start logs listener: %w", err)
	}

	// Start main bot loop
	errChan := make(chan error, 1)
	go func() {
		errChan <- a.run()
	}()

	a.logger.Info("🚀 Enhanced Pump.fun Bot started - listening for token discoveries!")

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		a.logger.LogShutdown(fmt.Sprintf("Received signal: %v", sig))
		a.shutdown()
		return nil
	case err := <-errChan:
		if err != nil {
			a.logger.WithError(err).Error("Bot error")
		}
		a.shutdown()
		return err
	}
}

// run is the main bot execution loop
func (a *App) run() error {
	a.logger.Info("Starting enhanced bot main loop...")

	// Get token events channel
	tokenChan := a.logsListener.GetTokenChannel()

	// Statistics ticker
	statsTicker := time.NewTicker(60 * time.Second)
	defer statsTicker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return nil

		case tokenEvent, ok := <-tokenChan:
			if !ok {
				a.logger.Warn("Token channel closed")
				return nil
			}

			a.tokensDiscovered++
			a.logger.WithFields(map[string]interface{}{
				"tokens_discovered":        a.tokensDiscovered,
				"mint":                     tokenEvent.Mint,
				"bonding_curve":            tokenEvent.BondingCurve,
				"associated_bonding_curve": tokenEvent.AssociatedBondingCurve,
				"name":                     tokenEvent.Name,
				"symbol":                   tokenEvent.Symbol,
				"creator":                  tokenEvent.Creator,
			}).Info("🎯 Processing discovered token")

			// Process the token event
			if err := a.processTokenEvent(tokenEvent); err != nil {
				a.logger.WithError(err).WithField("mint", tokenEvent.Mint).Error("Failed to process token event")
			}

		case <-statsTicker.C:
			a.logStatistics()
		}
	}
}

// processTokenEvent processes a discovered token event
func (a *App) processTokenEvent(event *pumpfun.TokenEvent) error {
	// Apply filters
	if !a.passesFilters(event) {
		a.logger.WithField("mint", event.Mint).Debug("Token filtered out")
		return nil
	}

	// If in logs-only mode, just log and return
	if *logsOnly {
		a.logger.WithFields(map[string]interface{}{
			"mint":                     event.Mint,
			"name":                     event.Name,
			"symbol":                   event.Symbol,
			"creator":                  event.Creator,
			"bonding_curve":            event.BondingCurve,
			"associated_bonding_curve": event.AssociatedBondingCurve,
		}).Info("📋 Token discovery (logs-only mode)")
		return nil
	}

	// If no trader (dry-run or no wallet), just log
	if a.trader == nil || *dryRun {
		a.logger.WithFields(map[string]interface{}{
			"mint":   event.Mint,
			"name":   event.Name,
			"symbol": event.Symbol,
		}).Info("📊 Would attempt to trade token (dry-run mode)")
		return nil
	}

	// Check if we should buy this token
	shouldBuy, reason := a.trader.ShouldBuyToken(a.ctx, event)
	if !shouldBuy {
		a.logger.WithFields(map[string]interface{}{
			"mint":   event.Mint,
			"reason": reason,
		}).Info("🚫 Skipping token")
		return nil
	}

	// Attempt to buy the token
	a.tradesAttempted++
	a.logger.WithField("mint", event.Mint).Info("💰 Attempting to buy token")

	result, err := a.trader.BuyToken(a.ctx, event)
	if err != nil {
		a.logger.WithError(err).WithField("mint", event.Mint).Error("❌ Trade failed")
		return fmt.Errorf("failed to buy token: %w", err)
	}

	if result.Success {
		a.tradesSuccessful++
		a.logger.WithFields(map[string]interface{}{
			"mint":          event.Mint,
			"signature":     result.Signature,
			"amount_sol":    result.AmountSOL,
			"amount_tokens": result.AmountTokens,
			"price":         result.Price,
		}).Info("✅ Trade successful")

		// If auto-sell is enabled and not in hold mode, schedule sell
		if a.config.Trading.AutoSell && !a.config.Strategy.HoldOnly && !*holdOnly {
			go a.scheduleSell(event.Mint.String(), result.AmountTokens)
		}
	} else {
		a.logger.WithFields(map[string]interface{}{
			"mint":  event.Mint,
			"error": result.Error,
		}).Error("❌ Trade failed")
	}

	return nil
}

// passesFilters checks if a token passes all configured filters
func (a *App) passesFilters(event *pumpfun.TokenEvent) bool {
	// Name pattern filter
	if a.config.Strategy.FilterByName && len(a.config.Strategy.NamePatterns) > 0 {
		nameMatch := false
		for _, pattern := range a.config.Strategy.NamePatterns {
			if strings.Contains(strings.ToLower(event.Name), strings.ToLower(pattern)) ||
				strings.Contains(strings.ToLower(event.Symbol), strings.ToLower(pattern)) {
				nameMatch = true
				a.logger.LogFilterMatch(event.Mint.String(), "name", pattern)
				break
			}
		}
		if !nameMatch {
			a.logger.LogFilterReject(event.Mint.String(), "name", "no pattern match")
			return false
		}
	}

	// Creator filter
	if a.config.Strategy.FilterByCreator && len(a.config.Strategy.AllowedCreators) > 0 {
		creatorMatch := false
		for _, allowedCreator := range a.config.Strategy.AllowedCreators {
			if event.Creator.String() == allowedCreator {
				creatorMatch = true
				a.logger.LogFilterMatch(event.Mint.String(), "creator", allowedCreator)
				break
			}
		}
		if !creatorMatch {
			a.logger.LogFilterReject(event.Mint.String(), "creator", "not in allowed list")
			return false
		}
	}

	// CLI match pattern filter
	if *matchPattern != "" {
		if !strings.Contains(strings.ToLower(event.Name), strings.ToLower(*matchPattern)) &&
			!strings.Contains(strings.ToLower(event.Symbol), strings.ToLower(*matchPattern)) {
			a.logger.LogFilterReject(event.Mint.String(), "cli_match", "pattern not found")
			return false
		}
		a.logger.LogFilterMatch(event.Mint.String(), "cli_match", *matchPattern)
	}

	// CLI creator filter
	if *creatorFilter != "" && event.Creator.String() != *creatorFilter {
		a.logger.LogFilterReject(event.Mint.String(), "cli_creator", "creator mismatch")
		return false
	}

	return true
}

// scheduleSell schedules a sell operation for a token
func (a *App) scheduleSell(mint string, amount uint64) {
	delay := time.Duration(a.config.Trading.SellDelaySeconds) * time.Second
	a.logger.WithFields(map[string]interface{}{
		"mint":  mint,
		"delay": delay,
	}).Info("⏰ Scheduling sell operation")

	time.Sleep(delay)

	// Calculate sell amount
	sellAmount := uint64(float64(amount) * (a.config.Trading.SellPercentage / 100.0))

	result, err := a.trader.SellToken(a.ctx, common.PublicKeyFromString(mint), sellAmount)
	if err != nil {
		a.logger.WithError(err).WithField("mint", mint).Error("❌ Scheduled sell failed")
		return
	}

	if result.Success {
		a.logger.WithFields(map[string]interface{}{
			"mint":       mint,
			"signature":  result.Signature,
			"amount_sol": result.AmountSOL,
		}).Info("✅ Scheduled sell successful")
	}
}

// testConnections tests all required connections
func (a *App) testConnections() error {
	a.logger.Info("Testing connections...")

	// Test RPC connection
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()

	slot, err := a.solanaClient.GetSlot(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to Solana RPC: %w", err)
	}

	a.logger.WithField("slot", slot).Info("✅ RPC connection successful")

	// Test wallet balance if wallet is available
	if a.wallet != nil {
		balance, err := a.wallet.GetBalanceSOL(ctx)
		if err != nil {
			return fmt.Errorf("failed to get wallet balance: %w", err)
		}

		a.logger.WithField("balance_sol", balance).Info("✅ Wallet connection successful")

		// Warn if balance is low
		if balance < 0.01 {
			a.logger.WithField("balance", balance).Warn("⚠️ Low wallet balance - consider adding more SOL")
		}
	}

	return nil
}

// logStatistics logs current bot statistics
func (a *App) logStatistics() {
	stats := map[string]interface{}{
		"tokens_discovered": a.tokensDiscovered,
		"trades_attempted":  a.tradesAttempted,
		"trades_successful": a.tradesSuccessful,
		"success_rate":      a.calculateSuccessRate(),
		"uptime":            time.Since(time.Now().Add(-time.Minute)).String(),
	}

	// Add listener stats
	//listenerStats := a.logsListener.GetStats()
	//for k, v := range listenerStats {
	//	stats["listener_"+k] = v
	//}

	// Add trading stats if trader is available
	if a.trader != nil {
		tradingStats := a.trader.GetTradingStats()
		for k, v := range tradingStats {
			stats["trading_"+k] = v
		}
	}

	a.logger.WithFields(stats).Info("📊 Bot Statistics")
}

// calculateSuccessRate calculates trade success rate
func (a *App) calculateSuccessRate() float64 {
	if a.tradesAttempted == 0 {
		return 0
	}
	return float64(a.tradesSuccessful) / float64(a.tradesAttempted) * 100
}

// shutdown gracefully shuts down the application
func (a *App) shutdown() {
	a.logger.Info("Shutting down enhanced bot...")

	// Cancel context to stop all operations
	a.cancel()

	// Stop logs listener
	if err := a.logsListener.Stop(); err != nil {
		a.logger.WithError(err).Error("Failed to stop logs listener")
	}

	// Stop trader if available
	if a.trader != nil {
		a.trader.Stop()
	}

	// Log final statistics
	a.logStatistics()

	// Log daily summary if we have any trades
	if err := a.tradeLogger.LogDailySummary(); err != nil {
		a.logger.WithError(err).Error("Failed to log daily summary")
	}

	a.logger.Info("Enhanced bot shutdown complete")
}

// Configuration and CLI handling functions (same as before)

func loadConfiguration() (*config.Config, error) {
	if *configPath != "" {
		return config.LoadConfig(*configPath)
	}

	// Try to load from default locations
	cfg, err := config.LoadConfig("")
	if err != nil {
		// If no config file found, use environment variables
		cfg = config.GetConfigFromEnv()
	}

	return cfg, nil
}

func overrideConfigWithFlags(cfg *config.Config) {
	if *yoloMode {
		cfg.Strategy.YoloMode = true
	}

	if *holdOnly {
		cfg.Strategy.HoldOnly = true
	}

	if *matchPattern != "" {
		cfg.Strategy.FilterByName = true
		cfg.Strategy.NamePatterns = []string{*matchPattern}
	}

	if *creatorFilter != "" {
		cfg.Strategy.FilterByCreator = true
		cfg.Strategy.AllowedCreators = []string{*creatorFilter}
	}

	if *network != "" {
		cfg.Network = *network
		cfg.RPCUrl = config.GetRPCEndpoint(*network)
		cfg.WSUrl = config.GetWSEndpoint(*network)
	}

	if *logLevel != "" {
		cfg.Logging.Level = *logLevel
	}

	if *dryRun {
		cfg.Trading.AutoSell = false // Disable auto-sell in dry-run
	}
}

func initializeLogger(cfg *config.Config) (*logger.Logger, error) {
	logConfig := logger.LogConfig{
		Level:       cfg.Logging.Level,
		Format:      cfg.Logging.Format,
		LogToFile:   cfg.Logging.LogToFile,
		LogFilePath: cfg.Logging.LogFilePath,
		TradeLogDir: cfg.Logging.TradeLogDir,
	}

	return logger.NewLogger(logConfig)
}

func validateFlags() error {
	if *network != "" && *network != "mainnet" && *network != "devnet" {
		return fmt.Errorf("invalid network: %s (must be 'mainnet' or 'devnet')", *network)
	}

	if *logLevel != "" {
		validLevels := []string{"debug", "info", "warn", "error"}
		valid := false
		for _, level := range validLevels {
			if strings.ToLower(*logLevel) == level {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid log level: %s (must be one of: %s)", *logLevel, strings.Join(validLevels, ", "))
		}
	}

	return nil
}

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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

const Version = "1.5.0"

// CLI flags
var (
	yoloMode       = flag.Bool("yolo", false, "Enable YOLO mode (continuous trading)")
	holdOnly       = flag.Bool("hold", false, "Hold only mode (no selling)")
	autoSell       = flag.Bool("auto-sell", false, "Enable automatic selling after purchase")
	sellDelay      = flag.Int("sell-delay", 30, "Delay before auto-sell in seconds")
	sellPercentage = flag.Float64("sell-percentage", 100.0, "Percentage of tokens to sell (100 = all)")
	closeATA       = flag.Bool("close-ata", true, "Close ATA account after selling to reclaim rent")

	useTokenAmount  = flag.Bool("use-tokens", true, "Use token amount instead of SOL amount")
	buyAmountTokens = flag.Uint64("buy-tokens", 1000000, "Amount of tokens to buy")
	buyAmountSOL    = flag.Float64("buy-sol", 0.01, "Amount of SOL to spend")

	matchPattern = flag.String("match", "", "Filter tokens by name pattern")
	network      = flag.String("network", "", "Network to use (mainnet/devnet)")
	logLevel     = flag.String("log-level", "", "Log level (debug/info/warn/error)")
	dryRun       = flag.Bool("dry-run", false, "Dry run mode (no actual trades)")
	configFile   = flag.String("config", "", "Path to config file")
	envFile      = flag.String("env", "", "Path to .env file")

	// Jito flags
	enableJito   = flag.Bool("jito", false, "Enable Jito MEV protection")
	jitoTip      = flag.Uint64("jito-tip", 10000, "Jito tip amount in lamports")
	jitoEndpoint = flag.String("jito-endpoint", "", "Custom Jito endpoint URL")

	// Enhanced performance flags
	skipValidation  = flag.Bool("skip-validation", false, "Skip validation checks for maximum speed")
	noConfirmation  = flag.Bool("no-confirm", false, "Don't wait for transaction confirmation")
	parallelWorkers = flag.Int("parallel-workers", 1, "Number of parallel processing workers")
	cacheBlockhash  = flag.Bool("cache-blockhash", true, "Cache blockhash for speed")
	fireAndForget   = flag.Bool("fire-and-forget", false, "Send transactions without waiting")

	// Performance flags
	logLatency = flag.Bool("log-latency", false, "Log detailed latency information")
	benchmark  = flag.Bool("benchmark", false, "Enable benchmark mode with detailed timing")
)

// Enhanced App with Ultra-Fast support only
type App struct {
	config       *config.Config
	logger       *logger.Logger
	tradeLogger  *logger.TradeLogger
	solanaClient *solana.Client
	wsClient     *solana.WSClient
	jitoClient   *solana.JitoClient
	wallet       *wallet.Wallet
	listener     *pumpfun.Listener
	trader       pumpfun.TraderInterface
	priceCalc    *pumpfun.PriceCalculator
	ctx          context.Context
	cancel       context.CancelFunc

	// Ultra-Fast Mode components
	tokenWorkers    []chan *pumpfun.TokenEvent
	processingStats *ProcessingStats
}

type ProcessingStats struct {
	TokensDiscovered    int64
	TokensProcessed     int64
	AverageLatency      float64
	FastestProcessingMs int64
	TotalProcessingTime time.Duration
	WorkerUtilization   map[int]float64
}

func main() {
	flag.Parse()

	// Apply CLI overrides to config
	cfg := loadConfigurationWithOverrides()

	// Initialize enhanced logger
	log := initializeEnhancedLogger(cfg)

	// Create and start application
	app, err := NewApp(cfg, log)
	if err != nil {
		log.WithError(err).Fatal("Failed to create application")
	}

	if err := app.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start application")
	}
}

func loadConfigurationWithOverrides() *config.Config {
	configPath := "configs/bot.yaml"
	if *configFile != "" {
		configPath = *configFile
	}

	cfg, err := config.LoadConfig(configPath, *envFile)
	if err != nil {
		fmt.Printf("Warning: Failed to load YAML config (%v), using environment variables only\n", err)
		cfg = config.GetConfigFromEnv(*envFile)
	}

	// Apply CLI overrides
	applyCliOverrides(cfg)

	return cfg
}

func applyCliOverrides(cfg *config.Config) {
	// Basic overrides
	if *network != "" {
		cfg.Network = *network
	}
	if *yoloMode {
		cfg.Strategy.YoloMode = true
	}
	if *holdOnly {
		cfg.Strategy.HoldOnly = true
		cfg.Trading.AutoSell = false // Disable auto-sell in hold mode
	}
	if *enableJito {
		cfg.JITO.Enabled = true
		cfg.JITO.UseForTrading = true
	}

	// Auto-sell overrides - UPDATED for milliseconds
	if *autoSell {
		cfg.Trading.AutoSell = true
	}
	if *sellPercentage != 100.0 {
		cfg.Trading.SellPercentage = *sellPercentage
	}
	if !*closeATA {
		cfg.Trading.CloseATAAfterSell = false
	}

	// NEW: Token-based trading overrides
	if *useTokenAmount {
		cfg.Trading.UseTokenAmount = true
	}
	if *buyAmountTokens != 1000000 {
		cfg.Trading.BuyAmountTokens = *buyAmountTokens
	}
	if *buyAmountSOL != 0.01 {
		cfg.Trading.BuyAmountSOL = *buyAmountSOL
	}

	// Ultra-Fast Mode is always enabled, but we can configure it
	cfg.UltraFast.Enabled = true
	cfg.UltraFast.CacheBlockhash = *cacheBlockhash

	if *skipValidation {
		cfg.UltraFast.SkipValidation = true
		cfg.Strategy.YoloMode = true // YOLO mode implies skip validation
	}

	// Auto-sell validation - UPDATED for milliseconds
	if cfg.Trading.AutoSell && cfg.Strategy.HoldOnly {
		log.Println("Warning: Auto-sell disabled because hold-only mode is enabled")
		cfg.Trading.AutoSell = false
	}

	if cfg.Trading.SellPercentage <= 0 || cfg.Trading.SellPercentage > 100 {
		log.Printf("Warning: Invalid sell percentage %.1f%%, using default 100%%", cfg.Trading.SellPercentage)
		cfg.Trading.SellPercentage = 100.0
	}

	// Validate sell delay
	if cfg.Trading.SellDelayMs < 0 {
		log.Printf("Warning: Invalid sell delay %dms, using default 1000ms", cfg.Trading.SellDelayMs)
		cfg.Trading.SellDelayMs = 1000
	}

	// Log warnings for very short delays
	if cfg.Trading.SellDelayMs < 100 {
		log.Printf("⚠️ WARNING: Very short sell delay (%dms) is extremely risky!", cfg.Trading.SellDelayMs)
	}
}

func initializeEnhancedLogger(cfg *config.Config) *logger.Logger {
	logConfig := logger.LogConfig{
		Level:       cfg.Logging.Level,
		Format:      cfg.Logging.Format,
		LogToFile:   cfg.Logging.LogToFile,
		LogFilePath: cfg.Logging.LogFilePath,
		TradeLogDir: cfg.Logging.TradeLogDir,
	}

	log, err := logger.NewLogger(logConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	return log
}

// Enhanced NewApp with Ultra-Fast Mode only
func NewApp(cfg *config.Config, log *logger.Logger) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize standard components...
	tradeLogger, err := logger.NewTradeLogger(cfg.Logging.TradeLogDir, log)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create trade logger: %w", err)
	}

	solanaClient := solana.NewClient(solana.ClientConfig{
		Endpoint: cfg.RPCUrl,
		APIKey:   cfg.RPCAPIKey,
		Timeout:  30 * time.Second,
	}, log.Logger)

	wsClient := solana.NewWSClient(cfg.WSUrl, log.Logger)

	// Initialize Jito client if enabled
	var jitoClient *solana.JitoClient
	if cfg.JITO.Enabled {
		jitoConfig := solana.JitoClientConfig{
			Endpoint: cfg.JITO.Endpoint,
			APIKey:   cfg.JITO.APIKey,
			Timeout:  30 * time.Second,
		}
		jitoClient = solana.NewJitoClient(jitoConfig, log.Logger)
	}

	// Initialize wallet
	var walletInstance *wallet.Wallet
	if !*dryRun && cfg.PrivateKey != "" {
		walletInstance, err = wallet.NewWallet(wallet.WalletConfig{
			PrivateKey: cfg.PrivateKey,
			Network:    cfg.Network,
		}, solanaClient, log.Logger, cfg)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create wallet: %w", err)
		}
	}

	// Initialize components
	priceCalc := pumpfun.NewPriceCalculator(solanaClient)
	listener := pumpfun.NewListener(wsClient, solanaClient, log, cfg)

	// Initialize trader - always ultra-fast mode
	var trader pumpfun.TraderInterface
	if !*dryRun && walletInstance != nil {
		trader = createTrader(cfg, walletInstance, solanaClient, jitoClient, log, tradeLogger)
	}

	app := &App{
		config:       cfg,
		logger:       log,
		tradeLogger:  tradeLogger,
		solanaClient: solanaClient,
		wsClient:     wsClient,
		jitoClient:   jitoClient,
		wallet:       walletInstance,
		listener:     listener,
		trader:       trader,
		priceCalc:    priceCalc,
		ctx:          ctx,
		cancel:       cancel,
		processingStats: &ProcessingStats{
			FastestProcessingMs: 999999,
			WorkerUtilization:   make(map[int]float64),
		},
	}

	// Initialize Ultra-Fast Mode workers if enabled
	if cfg.UltraFast.Enabled && cfg.UltraFast.ParallelWorkers > 1 {
		app.initializeUltraFastWorkers()
	}

	return app, nil
}

// createTrader creates ultra-fast trader with optional Jito protection
func createTrader(
	cfg *config.Config,
	wallet *wallet.Wallet,
	solanaClient *solana.Client,
	jitoClient *solana.JitoClient,
	log *logger.Logger,
	tradeLogger *logger.TradeLogger,
) pumpfun.TraderInterface {

	var traderType string
	if cfg.Trading.AutoSell {
		traderType = "ULTRA-FAST + AUTO-SELL"
	} else {
		traderType = "ULTRA-FAST"
	}

	log.WithFields(map[string]interface{}{
		"auto_sell_enabled": cfg.Trading.AutoSell,
		"sell_delay_ms":     cfg.Trading.SellDelayMs,
		"sell_percentage":   cfg.Trading.SellPercentage,
		"close_ata":         cfg.Trading.CloseATAAfterSell,
	}).Info("⚡⚡ Creating " + traderType + " trader")

	baseTrader := pumpfun.NewTrader(wallet, solanaClient, log, cfg)

	// Set trade logger for auto-sell functionality
	if tradeLogger != nil {
		baseTrader.SetTradeLogger(tradeLogger)
	}

	// Wrap with Jito protection if enabled
	if cfg.JITO.Enabled && cfg.JITO.UseForTrading {
		log.Info("🛡️ Wrapping trader with Jito MEV protection")
		jitoTrader := pumpfun.NewJitoTrader(baseTrader, jitoClient, wallet, log, cfg)

		// Set Jito client for auto-sell functionality
		if baseTrader.GetAutoSeller() != nil {
			baseTrader.SetJitoClient(jitoClient)
		}

		return jitoTrader
	}

	return baseTrader
}

// Enhanced Start method with Ultra-Fast Mode
func (a *App) Start() error {
	mode := "ULTRA-FAST"
	if a.config.UltraFast.ParallelWorkers > 1 {
		mode += fmt.Sprintf(" (%d workers)", a.config.UltraFast.ParallelWorkers)
	}

	if a.config.JITO.Enabled && a.config.JITO.UseForTrading {
		mode += " + JITO PROTECTED"
	}

	if a.config.Trading.AutoSell {
		mode += " + AUTO-SELL"
	}

	// NEW: Show trading mode
	tradingMode := "SOL-BASED"
	if a.config.IsTokenBasedTrading() {
		tradingMode = "TOKEN-BASED"
	}
	mode += fmt.Sprintf(" (%s)", tradingMode)

	a.logger.Info(fmt.Sprintf("🚀 Starting Pump.fun Bot v%s (%s MODE)", Version, mode))

	// Log auto-sell configuration - UPDATED for milliseconds
	if a.config.Trading.AutoSell {
		a.logger.WithFields(map[string]interface{}{
			"sell_delay_ms":     a.config.Trading.SellDelayMs, // CHANGED: now in milliseconds
			"sell_percentage":   a.config.Trading.SellPercentage,
			"close_ata":         a.config.Trading.CloseATAAfterSell,
			"auto_sell_enabled": true,
		}).Info("🤖 Auto-sell configuration active")

		// Warn about very short delays
		if a.config.Trading.SellDelayMs < 200 {
			a.logger.WithField("delay_ms", a.config.Trading.SellDelayMs).
				Warn("⚠️ Very short auto-sell delay - this is extremely risky!")
		}
	} else {
		a.logger.Info("🚫 Auto-sell is disabled - tokens will be held")
	}

	balance, err := a.wallet.GetBalanceSOL(a.ctx)
	if err != nil {
		return fmt.Errorf("failed to get balance: %w", err)
	}

	a.logger.Info(fmt.Sprintf("⚠️ Balance: %v SOL", balance))

	// NEW: Log trading configuration
	if a.config.IsTokenBasedTrading() {
		a.logger.WithFields(map[string]interface{}{
			"buy_amount_tokens": a.config.Trading.BuyAmountTokens,
			"trading_mode":      "token_based",
			"balance_checks":    "bypassed",
		}).Info("🪙 Token-based trading enabled")
	} else {
		a.logger.WithFields(map[string]interface{}{
			"buy_amount_sol": a.config.Trading.BuyAmountSOL,
			"trading_mode":   "sol_based",
			"balance_checks": "enabled",
		}).Info("💰 SOL-based trading enabled")
	}
	// Test connections and start components...
	if err := a.testConnections(); err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	// Start trader
	if starter, ok := a.trader.(interface{ Start() error }); ok {
		if err := starter.Start(); err != nil {
			return fmt.Errorf("failed to start trader: %w", err)
		}
	}

	// Start listener
	if err := a.listener.Start(); err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	// Start appropriate main loop
	errChan := make(chan error, 1)
	if a.config.UltraFast.Enabled && a.config.UltraFast.ParallelWorkers > 1 {
		go func() {
			errChan <- a.runUltraFastMode()
		}()
	} else {
		go func() {
			errChan <- a.runStandardMode()
		}()
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	a.logger.Info("🎯 Bot started - listening for new tokens!")
	a.logger.Info("⚡⚡ ULTRA-FAST MODE ACTIVE - Maximum speed processing enabled!")

	if a.config.Trading.AutoSell {
		a.logger.Info("🤖 AUTO-SELL MODE ACTIVE - Tokens will be automatically sold!")
	}

	// Wait for shutdown
	select {
	case sig := <-sigChan:
		a.logger.Info(fmt.Sprintf("🛑 Received signal: %v", sig))
		a.shutdown()
		return nil
	case err := <-errChan:
		a.shutdown()
		return err
	}
}

// initializeUltraFastWorkers sets up parallel processing workers
func (a *App) initializeUltraFastWorkers() {
	workerCount := a.config.UltraFast.ParallelWorkers
	bufferSize := workerCount * 2

	a.tokenWorkers = make([]chan *pumpfun.TokenEvent, workerCount)

	for i := 0; i < workerCount; i++ {
		workerChan := make(chan *pumpfun.TokenEvent, bufferSize)
		a.tokenWorkers[i] = workerChan

		// Start worker goroutine
		go a.ultraFastWorker(i, workerChan)
	}

	a.logger.WithField("workers", workerCount).Info("⚡⚡ Ultra-Fast workers initialized")
}

// ultraFastWorker processes tokens in parallel
func (a *App) ultraFastWorker(workerID int, tokenChan <-chan *pumpfun.TokenEvent) {
	processed := 0
	totalTime := time.Duration(0)

	for tokenEvent := range tokenChan {
		start := time.Now()

		// Process token with ultra-fast logic
		a.processTokenUltraFast(tokenEvent, workerID)

		// Update worker statistics
		processingTime := time.Since(start)
		processed++
		totalTime += processingTime

		if processed%10 == 0 {
			avgTime := totalTime / time.Duration(processed)
			a.processingStats.WorkerUtilization[workerID] = float64(avgTime.Milliseconds())
		}
	}
}

// processTokenUltraFast handles individual token processing with maximum speed
func (a *App) processTokenUltraFast(tokenEvent *pumpfun.TokenEvent, workerID int) {
	discoveryTime := time.Now()

	// Log discovery with worker ID
	a.logger.WithFields(map[string]interface{}{
		"mint":      tokenEvent.Mint.String(),
		"name":      tokenEvent.Name,
		"symbol":    tokenEvent.Symbol,
		"worker_id": workerID,
	}).Info("🎯 New token discovered")

	// Ultra-fast filtering (minimal checks)
	if !a.passesUltraFastFilters(tokenEvent) {
		return
	}

	// Execute trade immediately if in ultra-fast mode
	if a.config.UltraFast.SkipValidation {
		a.executeUltraFastTrade(tokenEvent, discoveryTime, workerID)
		return
	}

	// Standard validation (but still fast)
	if shouldBuy, reason := a.trader.ShouldBuyToken(a.ctx, tokenEvent); shouldBuy {
		a.executeUltraFastTrade(tokenEvent, discoveryTime, workerID)
	} else {
		a.logger.WithField("worker_id", workerID).Info("🚫 " + reason)
	}
}

// executeUltraFastTrade executes trade with detailed timing
func (a *App) executeUltraFastTrade(tokenEvent *pumpfun.TokenEvent, discoveryTime time.Time, workerID int) {
	tradeStart := time.Now()
	discoveryLag := tradeStart.Sub(discoveryTime)

	if a.config.UltraFast.LogLatency {
		a.logger.WithFields(map[string]interface{}{
			"worker_id":        workerID,
			"discovery_lag_ms": discoveryLag.Milliseconds(),
		}).Info("🛒 Executing ultra-fast trade")
	} else {
		a.logger.WithField("worker_id", workerID).Info("🛒 Executing buy transaction")
	}

	result, err := a.trader.BuyToken(a.ctx, tokenEvent)

	totalTime := time.Since(discoveryTime)
	tradeTime := time.Since(tradeStart)

	// Update statistics
	a.updateProcessingStats(totalTime, true)

	if err != nil {
		a.logger.WithError(err).WithFields(map[string]interface{}{
			"worker_id":        workerID,
			"total_time_ms":    totalTime.Milliseconds(),
			"trade_time_ms":    tradeTime.Milliseconds(),
			"discovery_lag_ms": discoveryLag.Milliseconds(),
		}).Error("❌ Ultra-fast trade failed")
	} else if result.Success {
		a.logger.WithFields(map[string]interface{}{
			"signature":        result.Signature,
			"amount":           result.AmountSOL,
			"worker_id":        workerID,
			"total_time_ms":    totalTime.Milliseconds(),
			"trade_time_ms":    tradeTime.Milliseconds(),
			"discovery_lag_ms": discoveryLag.Milliseconds(),
			"ultra_fast":       true,
		}).Info("⚡⚡ ULTRA-FAST TRADE SUCCESS")
	}
}

// passesUltraFastFilters applies minimal filtering for maximum speed
func (a *App) passesUltraFastFilters(event *pumpfun.TokenEvent) bool {
	// Only basic pattern matching for speed
	if a.config.Strategy.FilterByName && len(a.config.Strategy.NamePatterns) > 0 {
		eventName := strings.ToLower(event.Name)
		for _, pattern := range a.config.Strategy.NamePatterns {
			if strings.Contains(eventName, strings.ToLower(pattern)) {
				return true
			}
		}
		return false
	}
	return true
}

// updateProcessingStats tracks performance metrics
func (a *App) updateProcessingStats(processingTime time.Duration, success bool) {
	a.processingStats.TokensProcessed++
	a.processingStats.TotalProcessingTime += processingTime

	if processingTime.Milliseconds() < a.processingStats.FastestProcessingMs {
		a.processingStats.FastestProcessingMs = processingTime.Milliseconds()
	}

	// Update average (exponential moving average)
	if a.processingStats.TokensProcessed == 1 {
		a.processingStats.AverageLatency = float64(processingTime.Milliseconds())
	} else {
		alpha := 0.1
		oldAvg := a.processingStats.AverageLatency
		newValue := float64(processingTime.Milliseconds())
		a.processingStats.AverageLatency = oldAvg*(1-alpha) + newValue*alpha
	}
}

// runUltraFastMode handles parallel token processing
func (a *App) runUltraFastMode() error {
	tokenChan := a.listener.GetTokenChannel()
	statsTicker := time.NewTicker(30 * time.Second) // More frequent stats in ultra-fast mode
	defer statsTicker.Stop()

	workerIndex := 0

	for {
		select {
		case <-a.ctx.Done():
			return nil

		case tokenEvent, ok := <-tokenChan:
			if !ok {
				return nil
			}

			a.processingStats.TokensDiscovered++

			// Distribute tokens round-robin to workers
			select {
			case a.tokenWorkers[workerIndex] <- tokenEvent:
				workerIndex = (workerIndex + 1) % len(a.tokenWorkers)
			default:
				a.logger.Warn("⚠️ All workers busy, dropping token")
			}

		case <-statsTicker.C:
			a.logUltraFastStats()
		}
	}
}

// runStandardMode handles single-threaded processing
func (a *App) runStandardMode() error {
	tokenChan := a.listener.GetTokenChannel()
	statsTicker := time.NewTicker(60 * time.Second)
	defer statsTicker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return nil

		case tokenEvent, ok := <-tokenChan:
			if !ok {
				return nil
			}

			a.processTokenUltraFast(tokenEvent, 0)

		case <-statsTicker.C:
			// Standard stats logging
			a.logStandardStats()
		}
	}
}

// Enhanced logUltraFastStats with auto-sell statistics
func (a *App) logUltraFastStats() {
	stats := map[string]interface{}{
		"tokens_discovered":     a.processingStats.TokensDiscovered,
		"tokens_processed":      a.processingStats.TokensProcessed,
		"fastest_processing_ms": a.processingStats.FastestProcessingMs,
		"average_latency_ms":    a.processingStats.AverageLatency,
		"workers":               len(a.tokenWorkers),
	}

	// Add trader stats including auto-sell
	if a.trader != nil {
		traderStats := a.trader.GetTradingStats()
		for k, v := range traderStats {
			stats["trader_"+k] = v
		}
	}

	// Add worker utilization
	for workerID, utilization := range a.processingStats.WorkerUtilization {
		stats[fmt.Sprintf("worker_%d_avg_ms", workerID)] = utilization
	}

	a.logger.WithFields(stats).Info("⚡⚡ Ultra-Fast Performance Statistics (with Auto-Sell)")
}
func (a *App) logStandardStats() {
	// Standard statistics logging (existing implementation)
	a.logger.Info("📊 Standard Statistics")
}

// testConnections tests network connectivity
func (a *App) testConnections() error {
	// Test RPC connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := a.solanaClient.GetSlot(ctx)
	if err != nil {
		return fmt.Errorf("RPC connection test failed: %w", err)
	}

	a.logger.Info("✅ RPC connection test passed")

	return nil
}

func (a *App) shutdown() {
	a.logger.Info("🛑 Shutting down...")
	a.cancel()

	// Close worker channels
	for _, workerChan := range a.tokenWorkers {
		close(workerChan)
	}

	// Log final statistics
	if a.config.UltraFast.Enabled {
		a.logger.WithFields(map[string]interface{}{
			"total_discovered":      a.processingStats.TokensDiscovered,
			"total_processed":       a.processingStats.TokensProcessed,
			"fastest_processing_ms": a.processingStats.FastestProcessingMs,
			"average_latency_ms":    a.processingStats.AverageLatency,
		}).Info("📊 Final Ultra-Fast Statistics")
	}

	a.logger.Info("✅ Shutdown complete")
}

// cmd/bot/main.go
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

	"pump-fun-bot-go/internal/client"
	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/pumpfun"
	"pump-fun-bot-go/internal/wallet"
)

const Version = "2.0.0"

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

	network    = flag.String("network", "", "Network to use (mainnet/devnet)")
	dryRun     = flag.Bool("dry-run", false, "Dry run mode (no actual trades)")
	configFile = flag.String("config", "", "Path to config file")
	envFile    = flag.String("env", "", "Path to .env file")

	// Listener flags
	listenerType        = flag.String("listener", "", "Listener type (logs/blocks/multi)")
	enableLogListener   = flag.Bool("enable-logs", false, "Enable logs listener (for multi-listener)")
	enableBlockListener = flag.Bool("enable-blocks", false, "Enable blocks listener (for multi-listener)")

	// Performance flags
	skipValidation = flag.Bool("skip-validation", false, "Skip validation checks for maximum speed")
)

// App with listener factory integration
type App struct {
	config       *config.Config
	logger       *logger.Logger
	tradeLogger  *logger.TradeLogger
	solanaClient *client.Client
	wsClient     *client.WSClient
	wallet       *wallet.Wallet

	// Listener management
	listenerFactory *pumpfun.ListenerFactory
	listener        pumpfun.ListenerInterface

	trader    *pumpfun.Trader
	priceCalc *pumpfun.PriceCalculator
	ctx       context.Context
	cancel    context.CancelFunc

	// Ultra-Fast Mode components
	tokenWorkers    []chan *pumpfun.TokenEvent
	processingStats *ProcessingStats

	// Debug counters
	listenerTokensReceived int64
	mainChannelTokensSent  int64
	workerTokensReceived   int64

	// Channel monitoring
	channelMonitorTicker *time.Ticker
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

	// Initialize logger
	log := initializeLogger(cfg)

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

	// Listener overrides
	if *listenerType != "" {
		switch *listenerType {
		case "logs":
			cfg.Listener.Type = config.LogsListenerType
		case "blocks":
			cfg.Listener.Type = config.BlocksListenerType
		default:
			log.Printf("Warning: Invalid listener type '%s', using default", *listenerType)
		}
	}

	if *enableLogListener {
		cfg.Listener.EnableLogListener = true
	}
	if *enableBlockListener {
		cfg.Listener.EnableBlockListener = true
	}
	if *autoSell {
		cfg.Trading.AutoSell = true
	}
	if *sellPercentage != 100.0 {
		cfg.Trading.SellPercentage = *sellPercentage
	}
	if !*closeATA {
		cfg.Trading.CloseATAAfterSell = false
	}
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

	if *skipValidation {
		cfg.UltraFast.SkipValidation = true
		cfg.Strategy.YoloMode = true // YOLO mode implies skip validation
	}

	// Auto-sell validation
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

func initializeLogger(cfg *config.Config) *logger.Logger {
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

// NewApp creates a new application instance with listener factory
func NewApp(cfg *config.Config, log *logger.Logger) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize trade logger
	tradeLogger, err := logger.NewTradeLogger(cfg.Logging.TradeLogDir, log)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create trade logger: %w", err)
	}

	// Initialize Solana client
	solanaClient := client.NewClient(client.ClientConfig{
		RPCEndpoint:  cfg.RPCUrl,
		JITOEndpoint: cfg.Jito.BlockEngineURL,
		WSEndpoint:   cfg.WSUrl,
		APIKey:       cfg.RPCAPIKey,
		Timeout:      30 * time.Second,
	}, log.Logger)

	wsClient := client.NewWSClient(cfg.WSUrl, log.Logger)

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

	// Initialize listener factory
	listenerFactory := pumpfun.NewListenerFactory(wsClient, solanaClient, cfg, log)
	listener, err := listenerFactory.CreateStrategyListener()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	// Initialize components
	priceCalc := pumpfun.NewPriceCalculator(solanaClient)

	// Initialize trader (without Jito)
	var trader *pumpfun.Trader
	if !*dryRun && walletInstance != nil {
		trader = pumpfun.NewTrader(walletInstance, solanaClient, log, cfg)
		trader.SetTradeLogger(tradeLogger)
	}

	app := &App{
		config:          cfg,
		logger:          log,
		tradeLogger:     tradeLogger,
		solanaClient:    solanaClient,
		wsClient:        wsClient,
		wallet:          walletInstance,
		listenerFactory: listenerFactory,
		listener:        listener,
		trader:          trader,
		priceCalc:       priceCalc,
		ctx:             ctx,
		cancel:          cancel,
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

// Monitor channels for debugging
func (a *App) monitorChannels() {
	for {
		select {
		case <-a.channelMonitorTicker.C:
			a.logChannelStatus()
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *App) logChannelStatus() {
	// Get channel from listener
	tokenChan := a.listener.GetTokenChannel()

	// Check main channel status
	var mainChannelLen, mainChannelCap int
	if tokenChan != nil {
		mainChannelLen = len(tokenChan)
		mainChannelCap = cap(tokenChan)
	}

	// Check worker channels
	workerStats := make([]map[string]interface{}, len(a.tokenWorkers))
	totalWorkerBuffered := 0

	for i, workerChan := range a.tokenWorkers {
		if workerChan != nil {
			used := len(workerChan)
			capacity := cap(workerChan)
			totalWorkerBuffered += used

			workerStats[i] = map[string]interface{}{
				"worker_id":   i,
				"buffered":    used,
				"capacity":    capacity,
				"free":        capacity - used,
				"utilization": float64(used) / float64(capacity) * 100,
			}
		}
	}

	a.logger.WithFields(map[string]interface{}{
		"main_channel_len":         mainChannelLen,
		"main_channel_cap":         mainChannelCap,
		"listener_tokens_received": a.listenerTokensReceived,
		"main_channel_tokens_sent": a.mainChannelTokensSent,
		"worker_tokens_received":   a.workerTokensReceived,
		"total_worker_buffered":    totalWorkerBuffered,
		"worker_channels":          len(a.tokenWorkers),
		"worker_stats":             workerStats,
	}).Info("📊 Channel Status Monitor")

	// Warnings
	if a.listenerTokensReceived > 0 && a.mainChannelTokensSent == 0 {
		a.logger.Warn("⚠️ Listener receiving tokens but main channel not sending!")
	}

	if a.mainChannelTokensSent > 0 && a.workerTokensReceived == 0 {
		a.logger.Warn("⚠️ Main channel sending but workers not receiving!")
	}
}

// Start starts the application
func (a *App) Start() error {

	// Start channel monitoring
	a.channelMonitorTicker = time.NewTicker(10 * time.Second)
	go a.monitorChannels()

	mode := "ULTRA-FAST"
	if a.config.UltraFast.ParallelWorkers > 1 {
		mode += fmt.Sprintf(" (%d workers)", a.config.UltraFast.ParallelWorkers)
	}

	if a.config.Trading.AutoSell {
		mode += " + AUTO-SELL"
	}

	// Show listener type
	mode += fmt.Sprintf(" + %s-LISTENER", strings.ToUpper(string(a.config.GetListenerType())))

	// Show trading mode
	tradingMode := "SOL-BASED"
	if a.config.IsTokenBasedTrading() {
		tradingMode = "TOKEN-BASED"
	}
	mode += fmt.Sprintf(" (%s)", tradingMode)

	a.logger.Info(fmt.Sprintf("🚀 Starting Pump.fun Bot v%s (%s MODE)", Version, mode))

	// Log listener configuration
	a.logger.WithFields(map[string]interface{}{
		"listener_type":         a.config.GetListenerType(),
		"listener_buffer_size":  a.config.GetListenerBufferSize(),
		"enable_log_listener":   a.config.ShouldUseLogListener(),
		"enable_block_listener": a.config.ShouldUseBlockListener(),
		"duplicate_filter":      a.config.ShouldFilterDuplicates(),
		"filter_ttl_seconds":    a.config.Listener.DuplicateFilterTTL,
	}).Info("🎧 Listener configuration")

	// Log auto-sell configuration
	if a.config.Trading.AutoSell {
		a.logger.WithFields(map[string]interface{}{
			"sell_delay_ms":     a.config.Trading.SellDelayMs,
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

	// Get wallet balance if available
	if a.wallet != nil {
		balance, err := a.wallet.GetBalanceSOL(a.ctx)
		if err != nil {
			return fmt.Errorf("failed to get balance: %w", err)
		}
		a.logger.WithFields(map[string]interface{}{
			"wallet": a.wallet.GetPublicKey(),
		}).Info(fmt.Sprintf("💰 Balance: %v SOL", balance))
	}

	// Log trading configuration
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

	// Test connections and start components
	if err := a.testConnections(); err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	// Start trader
	if a.trader != nil {
		if err := a.trader.Start(); err != nil {
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

// Initialize ultra-fast workers
func (a *App) initializeUltraFastWorkers() {
	workerCount := a.config.UltraFast.ParallelWorkers
	bufferSize := a.config.UltraFast.TokenQueueSize

	// Ensure buffer size is not less than worker count
	if bufferSize < workerCount*2 {
		bufferSize = workerCount * 2
		a.logger.WithFields(map[string]interface{}{
			"original_size": a.config.UltraFast.TokenQueueSize,
			"adjusted_size": bufferSize,
			"workers":       workerCount,
		}).Info("📏 Adjusted buffer size for workers")
	}

	a.tokenWorkers = make([]chan *pumpfun.TokenEvent, workerCount)

	for i := 0; i < workerCount; i++ {
		workerChan := make(chan *pumpfun.TokenEvent, bufferSize)
		a.tokenWorkers[i] = workerChan

		// Start worker goroutine
		go a.ultraFastWorker(i, workerChan)

		a.logger.WithFields(map[string]interface{}{
			"worker_id":   i,
			"buffer_size": bufferSize,
		}).Debug("🔧 Ultra-Fast worker initialized")
	}

	a.logger.WithFields(map[string]interface{}{
		"workers":        workerCount,
		"buffer_size":    bufferSize,
		"total_capacity": workerCount * bufferSize,
	}).Info("⚡⚡ Ultra-Fast workers initialized")
}

// ultraFastWorker processes tokens in parallel
func (a *App) ultraFastWorker(workerID int, tokenChan <-chan *pumpfun.TokenEvent) {
	processed := 0
	totalTime := time.Duration(0)

	a.logger.WithField("worker_id", workerID).Info("🔧 Ultra-Fast worker started")

	for {
		select {
		case tokenEvent, ok := <-tokenChan:
			if !ok {
				a.logger.WithFields(map[string]interface{}{
					"worker_id": workerID,
					"processed": processed,
				}).Info("🔧 Ultra-Fast worker stopping")
				return
			}

			start := time.Now()

			a.logger.WithFields(map[string]interface{}{
				"worker_id": workerID,
				"mint":      tokenEvent.Mint.String(),
				"name":      tokenEvent.Name,
				"symbol":    tokenEvent.Symbol,
				"age_ms":    tokenEvent.GetAgeMs(),
			}).Info("🎯 Worker processing token")

			// Process token with ultra-fast logic
			a.processTokenUltraFast(tokenEvent, workerID)

			// Update worker statistics
			processingTime := time.Since(start)
			processed++
			totalTime += processingTime

			if processed%10 == 0 {
				avgTime := totalTime / time.Duration(processed)
				a.processingStats.WorkerUtilization[workerID] = float64(avgTime.Milliseconds())

				a.logger.WithFields(map[string]interface{}{
					"worker_id":       workerID,
					"processed":       processed,
					"avg_time_ms":     avgTime.Milliseconds(),
					"last_process_ms": processingTime.Milliseconds(),
				}).Debug("📊 Worker statistics")
			}

		case <-a.ctx.Done():
			a.logger.WithFields(map[string]interface{}{
				"worker_id": workerID,
				"processed": processed,
			}).Info("🔧 Ultra-Fast worker stopping (context done)")
			return
		}
	}
}

// processTokenUltraFast handles individual token processing with maximum speed
func (a *App) processTokenUltraFast(tokenEvent *pumpfun.TokenEvent, workerID int) {
	discoveryTime := time.Now()

	// Log discovery with worker ID and listener source
	a.logger.WithFields(map[string]interface{}{
		"mint":      tokenEvent.Mint,
		"name":      tokenEvent.Name,
		"symbol":    tokenEvent.Symbol,
		"worker_id": workerID,
		"source":    tokenEvent.Source,
		"confirmed": tokenEvent.IsConfirmed,
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
	if a.trader == nil {
		a.logger.WithField("worker_id", workerID).Debug("🔄 Dry run mode - no actual trade")
		return
	}

	tradeStart := time.Now()
	discoveryLag := tradeStart.Sub(discoveryTime)

	if a.config.UltraFast.LogLatency {
		a.logger.WithFields(map[string]interface{}{
			"worker_id":        workerID,
			"discovery_lag_ms": discoveryLag.Milliseconds(),
			"source":           tokenEvent.Source,
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
			"source":           tokenEvent.Source,
		}).Error("❌ Ultra-fast trade failed")
	} else if result.Success {
		a.logger.WithFields(map[string]interface{}{
			"signature":        result.Signature,
			"amount":           result.AmountSOL,
			"worker_id":        workerID,
			"total_time_ms":    totalTime.Milliseconds(),
			"trade_time_ms":    tradeTime.Milliseconds(),
			"discovery_lag_ms": discoveryLag.Milliseconds(),
			"source":           tokenEvent.Source,
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

func (a *App) runUltraFastMode() error {
	tokenChan := a.listener.GetTokenChannel()
	statsTicker := time.NewTicker(30 * time.Second)
	defer statsTicker.Stop()

	workerIndex := 0

	a.logger.WithFields(map[string]interface{}{
		"workers":     len(a.tokenWorkers),
		"buffer_size": a.config.UltraFast.TokenQueueSize,
	}).Info("⚡⚡ Ultra-Fast mode started - listening for tokens...")

	for {
		select {
		case <-a.ctx.Done():
			a.logger.Info("🛑 Ultra-Fast mode stopping...")
			return nil

		case tokenEvent, ok := <-tokenChan:
			if !ok {
				a.logger.Info("🔚 Token channel closed")
				return nil
			}

			a.processingStats.TokensDiscovered++

			a.logger.WithFields(map[string]interface{}{
				"mint":          tokenEvent.Mint.String(),
				"name":          tokenEvent.Name,
				"symbol":        tokenEvent.Symbol,
				"worker_target": workerIndex,
				"total_workers": len(a.tokenWorkers),
				"discovered":    a.processingStats.TokensDiscovered,
			}).Info("🎯 New token discovered - routing to worker")

			// Distribute tokens round-robin to workers
			select {
			case a.tokenWorkers[workerIndex] <- tokenEvent:
				a.logger.WithFields(map[string]interface{}{
					"worker_id": workerIndex,
					"mint":      tokenEvent.Mint.String(),
				}).Debug("✅ Token sent to worker")
				workerIndex = (workerIndex + 1) % len(a.tokenWorkers)

			default:
				// Worker busy, try next worker
				originalWorker := workerIndex
				for i := 0; i < len(a.tokenWorkers); i++ {
					workerIndex = (workerIndex + 1) % len(a.tokenWorkers)
					select {
					case a.tokenWorkers[workerIndex] <- tokenEvent:
						a.logger.WithFields(map[string]interface{}{
							"worker_id":       workerIndex,
							"original_worker": originalWorker,
							"mint":            tokenEvent.Mint.String(),
						}).Debug("✅ Token sent to alternate worker")
						goto tokenSent
					default:
						continue
					}
				}

				// All workers busy
				a.logger.WithFields(map[string]interface{}{
					"mint":         tokenEvent.Mint.String(),
					"workers_busy": len(a.tokenWorkers),
					"age_ms":       tokenEvent.GetAgeMs(),
				}).Warn("⚠️ All workers busy, dropping token")

			tokenSent:
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
			a.logStandardStats()
		}
	}
}

// logUltraFastStats logs performance statistics including listener stats
func (a *App) logUltraFastStats() {
	stats := map[string]interface{}{
		"tokens_discovered":     a.processingStats.TokensDiscovered,
		"tokens_processed":      a.processingStats.TokensProcessed,
		"fastest_processing_ms": a.processingStats.FastestProcessingMs,
		"average_latency_ms":    a.processingStats.AverageLatency,
		"workers":               len(a.tokenWorkers),
	}

	// Add trader stats if available
	if a.trader != nil {
		traderStats := a.trader.GetTradingStats()
		for k, v := range traderStats {
			stats["trader_"+k] = v
		}
	}

	// Add listener stats
	listenerStats := a.listener.GetStats()
	for k, v := range listenerStats {
		stats["listener_"+k] = v
	}

	// Add worker utilization
	for workerID, utilization := range a.processingStats.WorkerUtilization {
		stats[fmt.Sprintf("worker_%d_avg_ms", workerID)] = utilization
	}

	// Add blockhash cache info
	cacheInfo := a.solanaClient.GetBlockhashCacheInfo()
	for k, v := range cacheInfo {
		stats["blockhash_"+k] = v
	}

	a.logger.WithFields(stats).Info("⚡⚡ Ultra-Fast Performance Statistics")
}

func (a *App) logStandardStats() {
	stats := map[string]interface{}{
		"tokens_discovered": a.processingStats.TokensDiscovered,
		"tokens_processed":  a.processingStats.TokensProcessed,
	}

	// Add trader stats if available
	if a.trader != nil {
		traderStats := a.trader.GetTradingStats()
		for k, v := range traderStats {
			stats["trader_"+k] = v
		}
	}

	// Add listener stats
	listenerStats := a.listener.GetStats()
	for k, v := range listenerStats {
		stats["listener_"+k] = v
	}

	// Add blockhash cache info
	cacheInfo := a.solanaClient.GetBlockhashCacheInfo()
	for k, v := range cacheInfo {
		stats["blockhash_"+k] = v
	}

	a.logger.WithFields(stats).Info("📊 Standard Statistics")
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

	// Test blockhash caching
	cacheInfo := a.solanaClient.GetBlockhashCacheInfo()
	a.logger.WithFields(cacheInfo).Info("🔗 Blockhash cache status")

	return nil
}

func (a *App) shutdown() {
	a.logger.Info("🛑 Shutting down...")
	a.cancel()

	// Close worker channels
	for _, workerChan := range a.tokenWorkers {
		close(workerChan)
	}

	// Stop listener
	if a.listener != nil {
		a.listener.Stop()
	}

	// Stop trader
	if a.trader != nil {
		a.trader.Stop()
	}

	// Log final statistics
	if a.config.UltraFast.Enabled {
		finalStats := map[string]interface{}{
			"total_discovered":      a.processingStats.TokensDiscovered,
			"total_processed":       a.processingStats.TokensProcessed,
			"fastest_processing_ms": a.processingStats.FastestProcessingMs,
			"average_latency_ms":    a.processingStats.AverageLatency,
			"listener_type":         a.config.GetListenerType(),
		}

		// Add final listener stats
		if a.listener != nil {
			listenerStats := a.listener.GetStats()
			for k, v := range listenerStats {
				finalStats["final_listener_"+k] = v
			}
		}

		a.logger.WithFields(finalStats).Info("📊 Final Ultra-Fast Statistics")
	}

	// Log blockhash cache final status
	cacheInfo := a.solanaClient.GetBlockhashCacheInfo()
	a.logger.WithFields(cacheInfo).Info("🔗 Final blockhash cache status")

	a.logger.Info("✅ Shutdown complete")
}

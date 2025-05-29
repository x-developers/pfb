package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"pump-fun-bot-go/internal/config"
	"pump-fun-bot-go/internal/logger"
	"pump-fun-bot-go/internal/solana"
)

const (
	Version = "1.0.0"
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
)

// App represents the main application
type App struct {
	config       *config.Config
	logger       *logger.Logger
	tradeLogger  *logger.TradeLogger
	solanaClient *solana.Client
	ctx          context.Context
	cancel       context.CancelFunc
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

// NewApp creates a new application instance
func NewApp(cfg *config.Config, log *logger.Logger) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize trade logger
	tradeLogger, err := logger.NewTradeLogger(cfg.Logging.TradeLogDir, log)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create trade logger: %w", err)
	}

	// Initialize Solana client
	solanaClient := solana.NewClient(solana.ClientConfig{
		Endpoint: cfg.RPCUrl,
		APIKey:   cfg.RPCAPIKey,
		Timeout:  30 * time.Second,
	}, log.Logger)

	return &App{
		config:       cfg,
		logger:       log,
		tradeLogger:  tradeLogger,
		solanaClient: solanaClient,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start starts the application
func (a *App) Start() error {
	// Log startup information
	a.logger.LogStartup(Version, a.config.Network, a.config.RPCUrl, a.config)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Test Solana connection
	if err := a.testConnection(); err != nil {
		return fmt.Errorf("failed to connect to Solana: %w", err)
	}

	a.logger.Info("Bot started successfully - waiting for new tokens...")

	// Start main bot loop in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- a.run()
	}()

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
	a.logger.Info("Starting bot main loop...")

	// For now, just run a simple loop that tests the connection
	// In the next phase, we'll implement the actual trading logic
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return nil
		case <-ticker.C:
			// Test connection periodically
			if err := a.testConnection(); err != nil {
				a.logger.WithError(err).Warn("Connection test failed")
			} else {
				a.logger.Debug("Connection test successful")
			}
		}
	}
}

// testConnection tests the Solana RPC connection
func (a *App) testConnection() error {
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()

	// Test basic connectivity by getting current slot
	slot, err := a.solanaClient.GetSlot(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current slot: %w", err)
	}

	a.logger.WithField("slot", slot).Debug("Connected to Solana")
	return nil
}

// shutdown gracefully shuts down the application
func (a *App) shutdown() {
	a.logger.Info("Shutting down bot...")

	// Cancel context to stop all operations
	a.cancel()

	// Log daily summary if we have any trades
	if err := a.tradeLogger.LogDailySummary(); err != nil {
		a.logger.WithError(err).Error("Failed to log daily summary")
	}

	a.logger.Info("Bot shutdown complete")
}

// loadConfiguration loads the bot configuration
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

// overrideConfigWithFlags overrides configuration with CLI flags
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
}

// initializeLogger creates and configures the logger
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

// validateFlags validates CLI flags
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
